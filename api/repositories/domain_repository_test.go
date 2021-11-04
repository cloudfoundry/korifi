package repositories_test

import (
	"context"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("DomainRepository", func() {
	Describe("GetDomain", func() {
		var testCtx context.Context

		BeforeEach(func() {
			testCtx = context.Background()
		})

		When("multiple CFDomain resources exist", func() {
			var (
				cfDomain1 *networkingv1alpha1.CFDomain
				cfDomain2 *networkingv1alpha1.CFDomain
			)

			BeforeEach(func() {
				beforeCtx := context.Background()

				cfDomain1 = &networkingv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name: "domain-id-1",
					},
					Spec: networkingv1alpha1.CFDomainSpec{
						Name: "my-domain-1",
					},
				}
				Expect(k8sClient.Create(beforeCtx, cfDomain1)).To(Succeed())

				cfDomain2 = &networkingv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name: "domain-id-2",
					},
					Spec: networkingv1alpha1.CFDomainSpec{

						Name: "my-domain-2",
					},
				}
				Expect(k8sClient.Create(beforeCtx, cfDomain2)).To(Succeed())
			})

			It("fetches the CFDomain CR we're looking for", func() {
				domainRepo := DomainRepo{}
				client, err := BuildCRClient(k8sConfig)
				Expect(err).ToNot(HaveOccurred())

				domain := DomainRecord{}
				domain, err = domainRepo.FetchDomain(testCtx, client, "domain-id-1")
				Expect(err).ToNot(HaveOccurred())

				Expect(domain.GUID).To(Equal("domain-id-1"))
				Expect(domain.Name).To(Equal("my-domain-1"))
			})

			AfterEach(func() {
				afterCtx := context.Background()
				Expect(k8sClient.Delete(afterCtx, cfDomain1)).To(Succeed())
				Expect(k8sClient.Delete(afterCtx, cfDomain2)).To(Succeed())
			})
		})

		When("no CFDomain exists", func() {
			It("returns an error", func() {
				domainRepo := DomainRepo{}
				client, err := BuildCRClient(k8sConfig)
				Expect(err).ToNot(HaveOccurred())

				_, err = domainRepo.FetchDomain(testCtx, client, "non-existent-domain-guid")
				Expect(err).To(MatchError("Resource not found or permission denied."))
			})
		})
	})
})
