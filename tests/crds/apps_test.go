package crds_test

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers/config"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tests/helpers/fail_handler"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"code.cloudfoundry.org/korifi/tools/registry"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFApp", func() {
	var (
		app     *korifiv1alpha1.CFApp
		process *korifiv1alpha1.CFProcess
	)

	BeforeEach(func() {
		app = pushApp()

		Eventually(func(g Gomega) {
			processList := &korifiv1alpha1.CFProcessList{}
			g.Expect(k8sClient.List(ctx, processList, client.InNamespace(testSpace.Status.GUID))).To(Succeed())
			g.Expect(processList.Items).To(HaveLen(1))
			process = &processList.Items[0]
		}).Should(Succeed())
	})

	It("is in STOPPED state and has 0 processes", func() {
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)).To(Succeed())
			g.Expect(app.Status.ActualState).To(Equal(korifiv1alpha1.StoppedState))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(process), process)).To(Succeed())
			g.Expect(process.Status.ActualInstances).To(BeEquivalentTo(0))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			workloadList := &korifiv1alpha1.AppWorkloadList{}
			g.Expect(k8sClient.List(ctx, workloadList, client.InNamespace(testSpace.Status.GUID))).To(Succeed())
			g.Expect(workloadList.Items).To(BeEmpty())
		}).Should(Succeed())
	})

	When("the app is started", func() {
		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, k8sClient, app, func() {
				app.Spec.DesiredState = korifiv1alpha1.StartedState
			})).To(Succeed())
		})

		It("is in STARTED state and has 1 process", func() {
			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(app), app)).To(Succeed())
				g.Expect(app.Status.ActualState).To(Equal(korifiv1alpha1.StartedState))
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(process), process)).To(Succeed())
				g.Expect(process.Status.ActualInstances).To(BeEquivalentTo(1))
			}).Should(Succeed())

			Eventually(func(g Gomega) {
				workloadList := &korifiv1alpha1.AppWorkloadList{}
				g.Expect(k8sClient.List(ctx, workloadList, client.InNamespace(testSpace.Status.GUID))).To(Succeed())
				g.Expect(workloadList.Items).To(HaveLen(1))
				workload := workloadList.Items[0]
				g.Expect(workload.Finalizers).To(ContainElement(finalizer.AppWorkloadFinalizerName))
				g.Expect(workload.Status.ActualInstances).To(BeEquivalentTo(1))
			}).Should(Succeed())
		})
	})
})

func pushApp() *korifiv1alpha1.CFApp {
	GinkgoHelper()

	appEnvSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testSpace.Status.GUID,
			Name:      uuid.NewString(),
		},
		StringData: map[string]string{},
	}
	Expect(k8sClient.Create(ctx, &appEnvSecret)).To(Succeed())

	app := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testSpace.Status.GUID,
			Name:      uuid.NewString(),
		},
		Spec: korifiv1alpha1.CFAppSpec{
			DisplayName:  fmt.Sprintf("app-%d", time.Now().UnixMicro()),
			DesiredState: korifiv1alpha1.StoppedState,
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
			},
			EnvSecretName: appEnvSecret.Name,
		},
	}
	Expect(k8sClient.Create(ctx, app)).To(Succeed())

	appPackage := &korifiv1alpha1.CFPackage{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testSpace.Status.GUID,
			Name:      uuid.NewString(),
		},
		Spec: korifiv1alpha1.CFPackageSpec{
			Type: "bits",
			AppRef: corev1.LocalObjectReference{
				Name: app.Name,
			},
		},
	}
	Expect(k8sClient.Create(ctx, appPackage)).To(Succeed())

	uploadAppBits(app.Name, appPackage.Name)

	build := &korifiv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testSpace.Status.GUID,
			Name:      uuid.NewString(),
		},
		Spec: korifiv1alpha1.CFBuildSpec{
			PackageRef: corev1.LocalObjectReference{
				Name: appPackage.Name,
			},
			AppRef: corev1.LocalObjectReference{
				Name: app.Name,
			},
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: "buildpack",
			},
		},
	}
	Expect(k8sClient.Create(ctx, build)).To(Succeed())

	failHandler.RegisterFailHandler(fail_handler.Hook{
		Matcher: fail_handler.Always,
		Hook: func(config *rest.Config, _ fail_handler.TestFailure) {
			fail_handler.PrintBuildLogs(config, build.Name)
		},
	})

	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(build), build)).To(Succeed())
		g.Expect(meta.IsStatusConditionTrue(build.Status.Conditions, "Succeeded")).To(BeTrue())
	}).Should(Succeed())

	Expect(k8s.Patch(ctx, k8sClient, app, func() {
		app.Spec.CurrentDropletRef.Name = build.Name
	})).To(Succeed())

	return app
}

func uploadAppBits(appGUID, packageGUID string) {
	GinkgoHelper()

	Expect(k8sClient.Create(ctx, &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testSpace.Status.GUID,
			Name:      cfUser + "-admin",
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      cfUser,
			Namespace: rootNamespace,
		}},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: "korifi-controllers-admin",
		},
	})).To(Succeed())

	kpackBuilderConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "korifi",
			Name:      "korifi-kpack-image-builder-config",
		},
	}
	Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(kpackBuilderConfigMap), kpackBuilderConfigMap)).To(Succeed())

	kpackBuilderConfig := config.Config{}
	Expect(yaml.Unmarshal([]byte(kpackBuilderConfigMap.Data["config.yaml"]), &kpackBuilderConfig)).To(Succeed())
	repoCreator := registry.NewRepositoryCreator(kpackBuilderConfig.ContainerRegistryType)
	Expect(repoCreator.CreateRepository(ctx, fmt.Sprintf("%s%s-packages",
		kpackBuilderConfig.ContainerRepositoryPrefix,
		appGUID,
	))).To(Succeed())

	restClient := helpers.NewCorrelatedRestyClient(helpers.GetApiServerRoot(),
		func() string {
			return uuid.NewString()
		}).
		SetAuthScheme("Bearer").
		SetAuthToken(cfUserToken).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})

	resp, err := restClient.R().
		SetFiles(map[string]string{
			"bits": defaultAppBitsFile,
		}).Post("/v3/packages/" + packageGUID + "/upload")
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
}
