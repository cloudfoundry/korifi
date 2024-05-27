package integration_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	//+kubebuilder:scaffold:imports
)

var _ = Describe("RunnerInfosController", func() {
	var (
		ctx            context.Context
		runnerInfo     *korifiv1alpha1.RunnerInfo
		namespaceName  string
		runnerInfoName string
	)

	When("RunnerInfo is created with a matching runner", func() {
		var err error

		BeforeEach(func() {
			ctx = context.Background()
			runnerInfoName = "statefulset-runner"
			namespaceName = uuid.NewString()
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
				},
			})).To(Succeed())

			runnerInfo = &korifiv1alpha1.RunnerInfo{
				ObjectMeta: metav1.ObjectMeta{
					Name:      runnerInfoName,
					Namespace: namespaceName,
				},
				Spec: korifiv1alpha1.RunnerInfoSpec{
					RunnerName: runnerInfoName,
				},
			}
			Expect(k8sClient.Create(ctx, runnerInfo)).To(Succeed())
			Expect(err).NotTo(HaveOccurred())
		})

		getRunnerInfo := func(g Gomega) korifiv1alpha1.RunnerInfo {
			runnerInfo := korifiv1alpha1.RunnerInfo{}
			g.Eventually(func(g Gomega) {
				err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: namespaceName, Name: runnerInfoName}, &runnerInfo)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(runnerInfo.Status.ObservedGeneration).To(BeEquivalentTo(1))
			}).Should(Succeed())

			return runnerInfo
		}

		It("reconciles capabilities", func() {
			ri := getRunnerInfo(Default)
			Expect(ri.Status.Capabilities.RollingDeploy).To(BeTrue())
		})
	})

	When("RunnerInfo is created without a matching runner", func() {
		var err error

		BeforeEach(func() {
			ctx = context.Background()
			runnerInfoName = "foobrizzle-runner"
			namespaceName = uuid.NewString()
			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespaceName,
				},
			})).To(Succeed())

			runnerInfo = &korifiv1alpha1.RunnerInfo{
				ObjectMeta: metav1.ObjectMeta{
					Name:      runnerInfoName,
					Namespace: namespaceName,
				},
				Spec: korifiv1alpha1.RunnerInfoSpec{
					RunnerName: runnerInfoName,
				},
			}
			Expect(k8sClient.Create(ctx, runnerInfo)).To(Succeed())
			Expect(err).NotTo(HaveOccurred())
		})

		getRunnerInfo := func(g Gomega) korifiv1alpha1.RunnerInfo {
			runnerInfo := korifiv1alpha1.RunnerInfo{}
			g.Eventually(func(g Gomega) {
				err = k8sClient.Get(context.Background(), types.NamespacedName{Namespace: namespaceName, Name: runnerInfoName}, &runnerInfo)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(runnerInfo.Status.ObservedGeneration).To(BeEquivalentTo(0))
			}).Should(Succeed())

			return runnerInfo
		}

		It("does not reconcile capabilities", func() {
			ri := getRunnerInfo(Default)
			Expect(ri.Status.Capabilities.RollingDeploy).To(BeFalse())
		})
	})
})
