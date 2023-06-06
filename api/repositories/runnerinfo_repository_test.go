package repositories_test

import (
	"context"

	. "code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("RunnerInfoRepository", func() {
	BeforeEach(func() {
		createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
	})

	Describe("GetRunnerInfo", func() {
		When("a handler requests the RunnerInfo for the configured runner", func() {
			When("the runner supports rolling deploy", func() {
				var runnerInfoRepo *RunnerInfoRepository

				JustBeforeEach(func() {
					runnerInfoRepo = NewRunnerInfoRepository(userClientFactory, runnerName, rootNamespace)
					createRunnerInfoWithCleanup(ctx, runnerName, true)
				})

				It("returns the requested RunnerInfo", func() {
					runnerInfo, err := runnerInfoRepo.GetRunnerInfo(ctx, authInfo, runnerName)
					Expect(err).NotTo(HaveOccurred())
					Expect(runnerInfo.Name).To(Equal("statefulset-runner"))
					Expect(runnerInfo.RunnerName).To(Equal("statefulset-runner"))
				})

				It("it returns the runner capabilities", func() {
					runnerInfo, err := runnerInfoRepo.GetRunnerInfo(ctx, authInfo, runnerName)
					Expect(err).NotTo(HaveOccurred())
					Expect(runnerInfo.Capabilities.RollingDeploy).To(BeTrue())
				})
			})

			When("the runner does not support rolling deploy", func() {
				var noRollingDeployRunnerInfoRepo *RunnerInfoRepository
				noRollingDeployRunner := "rolling-deploy-not-supported"

				JustBeforeEach(func() {
					noRollingDeployRunnerInfoRepo = NewRunnerInfoRepository(userClientFactory, noRollingDeployRunner, rootNamespace)
					createRunnerInfoWithCleanup(ctx, noRollingDeployRunner, false)
				})

				It("it returns the runner capabilities", func() {
					runnerInfo, err := noRollingDeployRunnerInfoRepo.GetRunnerInfo(ctx, authInfo, noRollingDeployRunner)
					Expect(err).NotTo(HaveOccurred())
					Expect(runnerInfo.Capabilities.RollingDeploy).To(BeFalse())
				})
			})
		})
	})
})

func createRunnerInfoWithCleanup(ctx context.Context, name string, rollingDeploy bool) *korifiv1alpha1.RunnerInfo {
	runnerInfo := &korifiv1alpha1.RunnerInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: rootNamespace,
		},
		Spec: korifiv1alpha1.RunnerInfoSpec{
			RunnerName: name,
		},
	}
	Expect(k8sClient.Create(ctx, runnerInfo)).To(Succeed())

	runnerInfo.Status.Capabilities = korifiv1alpha1.RunnerInfoCapabilities{
		RollingDeploy: rollingDeploy,
	}
	Expect(k8sClient.Status().Update(ctx, runnerInfo)).To(Succeed())

	DeferCleanup(func() {
		Expect(k8sClient.Delete(ctx, runnerInfo)).To(Succeed())
	})

	return runnerInfo
}
