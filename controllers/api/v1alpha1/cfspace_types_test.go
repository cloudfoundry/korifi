package v1alpha1_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CF Space", func() {
	Describe("display name validation", func() {
		var (
			cfSpace   *korifiv1alpha1.CFSpace
			createErr error
		)

		BeforeEach(func() {
			cfOrg := &korifiv1alpha1.CFOrg{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFOrgSpec{
					DisplayName: uuid.NewString(),
				},
			}
			Expect(k8sClient.Create(ctx, cfOrg)).To(Succeed())

			Expect(k8sClient.Create(ctx, &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: cfOrg.Name},
			})).To(Succeed())

			cfSpace = &korifiv1alpha1.CFSpace{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cfOrg.Name,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFSpaceSpec{
					DisplayName: "space-name",
				},
			}
		})

		JustBeforeEach(func() {
			createErr = k8sClient.Create(ctx, cfSpace)
		})

		It("accepts a valid name", func() {
			Expect(createErr).NotTo(HaveOccurred())
		})

		When("name contains a space", func() {
			BeforeEach(func() {
				cfSpace.Spec.DisplayName = "hello there"
			})

			It("is allowed", func() {
				Expect(createErr).NotTo(HaveOccurred())
			})
		})

		When("display name contains disallowed characters", func() {
			BeforeEach(func() {
				cfSpace.Spec.DisplayName = "Nope\t\n\n"
			})

			It("fails", func() {
				Expect(createErr).To(HaveOccurred())
			})
		})
	})
})
