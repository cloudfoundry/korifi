package repositories_test

import (
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/api/v1alpha1"
	"context"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"

	"code.cloudfoundry.org/cf-k8s-api/repositories"

	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
)

var _ = SuiteDescribe("Domain API Shim", func(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	when("multiple CFDomain resources exist", func() {
		var (
			cfDomain1 *workloadsv1alpha1.CFDomain
			cfDomain2 *workloadsv1alpha1.CFDomain
		)

		it.Before(func() {
			ctx := context.Background()

			cfDomain1 = &workloadsv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name: "domain-id-1",
				},
				Spec: workloadsv1alpha1.CFDomainSpec{
					Name: "my-domain-1",
				},
			}
			g.Expect(k8sClient.Create(ctx, cfDomain1)).To(Succeed())

			cfDomain2 = &workloadsv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name: "domain-id-2",
				},
				Spec: workloadsv1alpha1.CFDomainSpec{

					Name: "my-domain-2",
				},
			}
			g.Expect(k8sClient.Create(ctx, cfDomain2)).To(Succeed())
		})

		it("fetches the CFDomain CR we're looking for", func() {
			domainRepo := repositories.DomainRepo{}
			domainClient, err := domainRepo.ConfigureClient(k8sConfig)
			g.Expect(err).ToNot(HaveOccurred())

			domain := repositories.DomainRecord{}
			domain, err = domainRepo.FetchDomain(domainClient, "domain-id-1")
			g.Expect(err).ToNot(HaveOccurred())

			g.Expect(domain.GUID).To(Equal("domain-id-1"))
			g.Expect(domain.Name).To(Equal("my-domain-1"))
		})

		it.After(func() {
			ctx := context.Background()
			g.Expect(k8sClient.Delete(ctx, cfDomain1)).To(Succeed())
			g.Expect(k8sClient.Delete(ctx, cfDomain2)).To(Succeed())
		})
	})

	when("no CFDomain exists", func() {
		it("returns an error", func() {
			domainRepo := repositories.DomainRepo{}
			domainClient, err := domainRepo.ConfigureClient(k8sConfig)
			g.Expect(err).ToNot(HaveOccurred())

			_, err = domainRepo.FetchDomain(domainClient, "non-existent-domain-guid")
			g.Expect(err).To(MatchError("not found"))
		})
	})
})
