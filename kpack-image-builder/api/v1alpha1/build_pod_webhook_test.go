package v1alpha1_test

import (
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Buildpack Pod Mutating Webhook", func() {
	var (
		namespace string
		pod       *corev1.Pod
	)

	BeforeEach(func() {
		namespace = testutils.PrefixedGUID("ns")
		err := k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		})
		Expect(err).NotTo(HaveOccurred())

		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testutils.PrefixedGUID("pod"),
				Namespace: namespace,
			},
			Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{{
					Name:    "init-1",
					Image:   "alpine",
					Command: []string{"sleep", "1234"},
				}},
				Containers: []corev1.Container{{
					Name:    "c-1",
					Image:   "alpine",
					Command: []string{"sleep", "9876"},
				}},
			},
		}
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(ctx, pod)).To(Succeed())
		lookupKey := client.ObjectKeyFromObject(pod)
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(ctx, lookupKey, pod)).To(Succeed())
		}).Should(Succeed())
	})

	checkSecurityContext := func(securityContext *corev1.SecurityContext) {
		ExpectWithOffset(1, securityContext).NotTo(BeNil())

		ExpectWithOffset(1, securityContext.AllowPrivilegeEscalation).To(PointTo(BeFalse()))
		ExpectWithOffset(1, securityContext.RunAsNonRoot).To(PointTo(BeTrue()))

		ExpectWithOffset(1, securityContext.Capabilities).NotTo(BeNil())
		ExpectWithOffset(1, securityContext.Capabilities.Drop).To(ConsistOf(corev1.Capability("ALL")))
		ExpectWithOffset(1, securityContext.Capabilities.Add).To(BeEmpty())

		ExpectWithOffset(1, securityContext.SeccompProfile).NotTo(BeNil())
		ExpectWithOffset(1, securityContext.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))
	}

	It("adds security context to containers", func() {
		checkSecurityContext(pod.Spec.Containers[0].SecurityContext)
	})

	It("adds security context to init containers", func() {
		checkSecurityContext(pod.Spec.InitContainers[0].SecurityContext)
	})
})
