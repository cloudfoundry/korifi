package v1alpha1_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CF Org", func() {
	Describe("display name validation", func() {
		var (
			cfOrg     *korifiv1alpha1.CFOrg
			createErr error
		)

		BeforeEach(func() {
			cfOrg = &korifiv1alpha1.CFOrg{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFOrgSpec{
					DisplayName: "org-name-" + uuid.NewString(),
				},
			}
		})

		JustBeforeEach(func() {
			createErr = k8sClient.Create(ctx, cfOrg)
		})

		It("accepts a valid name", func() {
			Expect(createErr).NotTo(HaveOccurred())
		})

		When("an org with the same display name already exists", func() {
			BeforeEach(func() {
				Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFOrg{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: namespace,
						Name:      uuid.NewString(),
					},
					Spec: korifiv1alpha1.CFOrgSpec{
						DisplayName: cfOrg.Spec.DisplayName,
					},
				})).To(Succeed())
			})

			It("fails", func() {
				Expect(createErr).To(HaveOccurred())
			})
		})

		When("name contains a space", func() {
			BeforeEach(func() {
				cfOrg.Spec.DisplayName = "hello there"
			})

			It("is allowed", func() {
				Expect(createErr).NotTo(HaveOccurred())
			})
		})

		When("display name contains disallowed characters", func() {
			BeforeEach(func() {
				cfOrg.Spec.DisplayName = "Nope\t\n\n"
			})

			It("fails", func() {
				Expect(createErr).To(HaveOccurred())
			})
		})
	})
})
