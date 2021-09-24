package repositories_test

import (
	"context"
	"testing"

	. "code.cloudfoundry.org/cf-k8s-api/repositories"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = SuiteDescribe("Domain API Shim", func(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	var testCtx context.Context

	it.Before(func() {
		testCtx = context.Background()
	})

	when("multiple CFDomain resources exist", func() {
		var (
			cfDomain1 *networkingv1alpha1.CFDomain
			cfDomain2 *networkingv1alpha1.CFDomain
		)

		it.Before(func() {
			beforeCtx := context.Background()

			cfDomain1 = &networkingv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name: "domain-id-1",
				},
				Spec: networkingv1alpha1.CFDomainSpec{
					Name: "my-domain-1",
				},
			}
			g.Expect(k8sClient.Create(beforeCtx, cfDomain1)).To(Succeed())

			cfDomain2 = &networkingv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name: "domain-id-2",
				},
				Spec: networkingv1alpha1.CFDomainSpec{

					Name: "my-domain-2",
				},
			}
			g.Expect(k8sClient.Create(beforeCtx, cfDomain2)).To(Succeed())
		})

		it("fetches the CFDomain CR we're looking for", func() {
			domainRepo := DomainRepo{}
			client, err := BuildClient(k8sConfig)
			g.Expect(err).ToNot(HaveOccurred())

			domain := DomainRecord{}
			domain, err = domainRepo.FetchDomain(testCtx, client, "domain-id-1")
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(domain.GUID).To(Equal("domain-id-1"))
			g.Expect(domain.Name).To(Equal("my-domain-1"))
		})

		it.After(func() {
			afterCtx := context.Background()
			g.Expect(k8sClient.Delete(afterCtx, cfDomain1)).To(Succeed())
			g.Expect(k8sClient.Delete(afterCtx, cfDomain2)).To(Succeed())
		})
	})

	when("no CFDomain exists", func() {
		it("returns an error", func() {
			domainRepo := DomainRepo{}
			client, err := BuildClient(k8sConfig)
			g.Expect(err).ToNot(HaveOccurred())

			_, err = domainRepo.FetchDomain(testCtx, client, "non-existent-domain-guid")
			g.Expect(err).To(MatchError("Resource not found or permission denied."))
		})
	})
})
