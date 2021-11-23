package integration_test

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads/testutils"

	eiriniv1 "code.cloudfoundry.org/eirini-controller/pkg/apis/eirini/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFProcessReconciler Integration Tests", func() {
	var (
		testNamespace string
		ns            *corev1.Namespace

		testProcessGUID string
		testAppGUID     string
		testBuildGUID   string
		testPackageGUID string
		cfProcess       *workloadsv1alpha1.CFProcess
		cfPackage       *workloadsv1alpha1.CFPackage
		cfApp           *workloadsv1alpha1.CFApp
		cfBuild         *workloadsv1alpha1.CFBuild
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
		ctx := context.Background()

		testNamespace = GenerateGUID()
		ns = createNamespace(ctx, k8sClient, testNamespace)

		testAppGUID = GenerateGUID()
		testProcessGUID = GenerateGUID()
		testBuildGUID = GenerateGUID()
		testPackageGUID = GenerateGUID()

		// Technically the app controller should be creating this process based on CFApp and CFBuild, but we
		// want to drive testing with a specific CFProcess instead of cascading (non-object-ref) state through
		// other resources.
		cfProcess = BuildCFProcessCRObject(testProcessGUID, testNamespace, testAppGUID, processTypeWeb, processTypeWebCommand)
		Expect(
			k8sClient.Create(ctx, cfProcess),
		).To(Succeed())

		cfApp = BuildCFAppCRObject(testAppGUID, testNamespace)
		UpdateCFAppWithCurrentDropletRef(cfApp, testBuildGUID)
		cfApp.Spec.EnvSecretName = testAppGUID + "-env"

		appEnvSecret := BuildCFAppEnvVarsSecret(testAppGUID, testNamespace, map[string]string{"test-env-key": "test-env-val"})
		Expect(
			k8sClient.Create(ctx, appEnvSecret),
		).To(Succeed())

		cfPackage = BuildCFPackageCRObject(testPackageGUID, testNamespace, testAppGUID)
		Expect(
			k8sClient.Create(ctx, cfPackage),
		).To(Succeed())
		cfBuild = BuildCFBuildObject(testBuildGUID, testNamespace, testPackageGUID, testAppGUID)
		dropletProcessTypeMap := map[string]string{
			processTypeWeb:    processTypeWebCommand,
			processTypeWorker: processTypeWorkerCommand,
		}
		dropletPorts := []int32{port8080, port9000}
		buildDropletStatus := BuildCFBuildDropletStatusObject(dropletProcessTypeMap, dropletPorts)
		createBuildWithDroplet(ctx, k8sClient, cfBuild, buildDropletStatus)
	})

	AfterEach(func() {
		k8sClient.Delete(context.Background(), ns)
	})

	When("the CFApp desired state is STARTED", func() {
		BeforeEach(func() {
			ctx := context.Background()
			cfApp.Spec.DesiredState = workloadsv1alpha1.StartedState
			Expect(
				k8sClient.Create(ctx, cfApp),
			).To(Succeed())
		})

		It("eventually reconciles the CFProcess into an LRP", func() {
			ctx := context.Background()
			var lrp eiriniv1.LRP

			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testProcessGUID, Namespace: testNamespace}, &lrp)
				Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get LRP/%s in namespace %s", testProcessGUID, testNamespace))
				return lrp.GetName()
			}, 5*time.Second).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for LRP/%s in namespace %s to be created", testProcessGUID, testNamespace))

			Expect(lrp.OwnerReferences).To(HaveLen(1), "expected length of ownerReferences to be 1")
			Expect(lrp.OwnerReferences[0].Name).To(Equal(cfProcess.Name))

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

	When("the CFProcess has health check of type process", func() {
		BeforeEach(func() {
			ctx := context.Background()

			cfProcess.Spec.HealthCheck.Type = "process"
			cfProcess.Spec.Ports = []int32{}
			Expect(
				k8sClient.Update(ctx, cfProcess),
			).To(Succeed())

			cfApp.Spec.DesiredState = workloadsv1alpha1.StartedState
			Expect(
				k8sClient.Create(ctx, cfApp),
			).To(Succeed())
		})

		It("eventually reconciles the CFProcess into an LRP", func() {
			ctx := context.Background()
			var lrp eiriniv1.LRP

			Eventually(func() string {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: testProcessGUID, Namespace: testNamespace}, &lrp)
				Expect(client.IgnoreNotFound(err)).NotTo(HaveOccurred(), fmt.Sprintf("Failed to get LRP/%s in namespace %s", testProcessGUID, testNamespace))
				return lrp.GetName()
			}, 5*time.Second).ShouldNot(BeEmpty(), fmt.Sprintf("Timed out waiting for LRP/%s in namespace %s to be created", testProcessGUID, testNamespace))

			Expect(lrp.Spec.Health.Type).To(Equal(string(cfProcess.Spec.HealthCheck.Type)))
			Expect(lrp.Spec.Health.Port).To(BeZero())
		})
	})
})
