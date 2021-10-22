package integration_test

import (
	"context"
	"fmt"
	"time"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/workloads/testutils"

	eiriniv1 "code.cloudfoundry.org/eirini-controller/pkg/apis/eirini/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFProcessReconciler Integration Tests", func() {
	var (
		testNamespace   string
		testProcessGUID string
		testAppGUID     string
		testBuildGUID   string
		testPackageGUID string

		ns *corev1.Namespace

		cfProcess *workloadsv1alpha1.CFProcess
		cfPackage *workloadsv1alpha1.CFPackage
		cfApp     *workloadsv1alpha1.CFApp
		cfBuild   *workloadsv1alpha1.CFBuild
	)

	const (
		defaultEventuallyTimeoutSeconds = 2
		processTypeWeb                  = "web"
		processTypeWebCommand           = "bundle exec rackup config.ru -p $PORT -o 0.0.0.0"
		processTypeWorker               = "worker"
		processTypeWorkerCommand        = "bundle exec rackup config.ru"
		port8080                        = 8080
		port9000                        = 9000
	)

	BeforeEach(func() {
		var err error
		ctx := context.Background()

		testNamespace = GenerateGUID()
		Expect(err).NotTo(HaveOccurred())

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())

		testAppGUID = GenerateGUID()
		testProcessGUID = GenerateGUID()
		testBuildGUID = GenerateGUID()
		testPackageGUID = GenerateGUID()
		testAppEnvSecretName := GenerateGUID()

		// Technically the app controller should be creating this process based on CFApp and CFBuild, but we
		// want to drive testing with a specific CFProcess instead of cascading (non-object-ref) state through
		// other resources.
		cfProcess = BuildCFProcessCRObject(testProcessGUID, testNamespace, testAppGUID, processTypeWeb, processTypeWebCommand)
		Expect(k8sClient.Create(ctx, cfProcess)).To(Succeed())

		cfApp = BuildCFAppCRObject(testAppGUID, testNamespace)
		UpdateCFAppWithCurrentDropletRef(cfApp, testBuildGUID)
		cfApp.Spec.EnvSecretName = testAppEnvSecretName
		cfApp.Spec.DesiredState = workloadsv1alpha1.StartedState
		Expect(k8sClient.Create(ctx, cfApp)).To(Succeed())

		appEnvSecret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      testAppEnvSecretName,
			},
			StringData: map[string]string{
				"test-env-key": "test-env-val",
			},
		}
		Expect(k8sClient.Create(ctx, &appEnvSecret)).To(Succeed())

		cfPackage = BuildCFPackageCRObject(testPackageGUID, testNamespace, testAppGUID)
		Expect(k8sClient.Create(ctx, cfPackage)).To(Succeed())

		cfBuild = BuildCFBuildObject(testBuildGUID, testNamespace, testPackageGUID, testAppGUID)
		Expect(k8sClient.Create(ctx, cfBuild)).To(Succeed())
		cfBuildLookupKey := types.NamespacedName{Name: testBuildGUID, Namespace: testNamespace}
		Eventually(func() []metav1.Condition {
			err := k8sClient.Get(ctx, cfBuildLookupKey, cfBuild)
			if err != nil {
				return nil
			}
			return cfBuild.Status.Conditions
		}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty(), "could not retrieve the cfbuild")

		cfBuild.Status.BuildDropletStatus = &workloadsv1alpha1.BuildDropletStatus{
			Registry: workloadsv1alpha1.Registry{
				Image:            "image/registry/url",
				ImagePullSecrets: nil,
			},
			Stack: "cflinuxfs3",
			ProcessTypes: []workloadsv1alpha1.ProcessType{
				{
					Type:    processTypeWeb,
					Command: processTypeWebCommand,
				},
				{
					Type:    processTypeWorker,
					Command: processTypeWorkerCommand,
				},
			},
			Ports: []int32{port8080, port9000},
		}
		Expect(k8sClient.Status().Update(ctx, cfBuild)).To(Succeed())
	})

	AfterEach(func() {
		ctx := context.Background()

		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cfApp))).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cfBuild))).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cfPackage))).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cfProcess))).To(Succeed())
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, ns))).To(Succeed())
	})

	When("a CFProcess is created", func() {
		When("the CFApp desired state is STARTED", func() {
			It("eventually reconciles the CFProcess into an LRP", func() {
				ctx := context.Background()
				var lrp eiriniv1.LRP

				Eventually(func() string {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: testProcessGUID, Namespace: testNamespace}, &lrp)
					Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get LRP/%s in namespace %s", testProcessGUID, testNamespace))
					return lrp.GetName()
				}, defaultEventuallyTimeoutSeconds*time.Second).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for LRP/%s in namespace %s to be created", testProcessGUID, testNamespace))

				Expect(lrp.Spec.GUID).To(Equal(cfProcess.Name), "Expected lrp spec GUID to match cfProcess GUID")
				Expect(lrp.Spec.DiskMB).To(Equal(cfProcess.Spec.DiskQuotaMB), "lrp DiskMB does not match")
				Expect(lrp.Spec.MemoryMB).To(Equal(cfProcess.Spec.MemoryMB), "lrp MemoryMB does not match")
				Expect(lrp.Spec.Image).To(Equal(cfBuild.Status.BuildDropletStatus.Registry.Image), "lrp Image does not match Droplet")
				Expect(lrp.Spec.ProcessType).To(Equal(processTypeWeb), "lrp process type does not match")
				Expect(lrp.Spec.AppName).To(Equal(cfApp.Spec.Name), "lrp app name does not match CFApp")
				Expect(lrp.Spec.AppGUID).To(Equal(cfApp.Name), "lrp app GUID does not match CFApp")
				Expect(lrp.Spec.Ports).To(Equal(cfProcess.Spec.Ports), "lrp ports do not match")
				Expect(lrp.Spec.Instances).To(Equal(cfProcess.Spec.DesiredInstances), "lrp desired instances does not match CFApp")
				Expect(lrp.Spec.CPUWeight).To(BeZero(), "expected cpu to always be 0")
				Expect(lrp.Spec.Sidecars).To(BeNil(), "expected sidecars to always be nil")
				Expect(lrp.Spec.Env).To(HaveKeyWithValue("test-env-key", "test-env-val"))
				Expect(lrp.Spec.Command).To(ConsistOf("/cnb/lifecycle/launcher", processTypeWebCommand))
			})

			It("sets the LRP ownerRef to the CFProcess", func() {
				ctx := context.Background()
				var lrp eiriniv1.LRP

				Eventually(func() string {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: testProcessGUID, Namespace: testNamespace}, &lrp)
					Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get LRP/%s in namespace %s", testProcessGUID, testNamespace))
					return lrp.GetName()
				}, defaultEventuallyTimeoutSeconds*time.Second).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for LRP/%s in namespace %s to be created", testProcessGUID, testNamespace))

				Expect(lrp.OwnerReferences).To(HaveLen(1), "expected length of ownerReferences to be 1")
				Expect(lrp.OwnerReferences[0].Name).To(Equal(cfProcess.Name))
			})
		})
	})

	// We need to actually listen on CFApp desiredStatus updates; currently only changes to the CFProcess trigger reconcile requests
	When("a CFProcess exists and", func() {
		When("a CFApp desired state is updated to STOPPED", func() {
			BeforeEach(func() {
				ctx := context.Background()

				// Wait for LRP to exist before updating CFApp
				Eventually(func() string {
					var lrp eiriniv1.LRP
					err := k8sClient.Get(ctx, types.NamespacedName{Name: testProcessGUID, Namespace: testNamespace}, &lrp)
					Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get LRP/%s in namespace %s", testProcessGUID, testNamespace))
					return lrp.GetName()
				}, defaultEventuallyTimeoutSeconds*time.Second).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for LRP/%s in namespace %s to be created", testProcessGUID, testNamespace))

				originalCFApp := cfApp.DeepCopy()
				cfApp.Spec.DesiredState = workloadsv1alpha1.StoppedState
				Expect(k8sClient.Patch(ctx, cfApp, client.MergeFrom(originalCFApp))).To(Succeed())
			})

			It("eventually deletes the LRPs", func() {
				ctx := context.Background()

				Eventually(func() bool {
					var lrp eiriniv1.LRP
					err := k8sClient.Get(ctx, types.NamespacedName{Name: testProcessGUID, Namespace: testNamespace}, &lrp)
					return apierrors.IsNotFound(err)
				}, defaultEventuallyTimeoutSeconds*time.Second).Should(BeTrue(), "Timed out waiting for deletion of LRP/%s in namespace %s to cause NotFound error", testProcessGUID, testNamespace)
			})
		})
	})
})
