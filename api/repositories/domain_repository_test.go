package repositories_test

import (
	"context"
	"fmt"
	"time"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("DomainRepository", func() {
	Describe("FetchDomain", func() {
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
				client, err := BuildPrivilegedClient(k8sConfig, "")
				Expect(err).ToNot(HaveOccurred())

				domainRepo := NewDomainRepo(client)

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
				client, err := BuildPrivilegedClient(k8sConfig, "")
				Expect(err).ToNot(HaveOccurred())

				domainRepo := NewDomainRepo(client)

				_, err = domainRepo.FetchDomain(testCtx, client, "non-existent-domain-guid")
				Expect(err).To(MatchError("Resource not found or permission denied."))
			})
		})
	})

	Describe("FetchDomainList", func() {
		var (
			testCtx context.Context

			domainRepo        *DomainRepo
			domainClient      client.Client
			domainListMessage DomainListMessage
		)

		BeforeEach(func() {
			testCtx = context.Background()

			domainListMessage = DomainListMessage{}
			var err error
			domainClient, err = BuildPrivilegedClient(k8sConfig, "")
			Expect(err).ToNot(HaveOccurred())

			domainRepo = NewDomainRepo(domainClient)
		})

		When("multiple CFDomain exists and no filter is provided", func() {
			const (
				domainName1 = "my-domain-name-1"
				domainName2 = "my-domain-name-2"
			)
			var (
				domainGUID1 string
				domainGUID2 string

				cfDomain1 *networkingv1alpha1.CFDomain
				cfDomain2 *networkingv1alpha1.CFDomain
			)

			BeforeEach(func() {
				domainGUID1 = generateGUID()
				domainGUID2 = generateGUID()

				ctx := context.Background()

				cfDomain1 = &networkingv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name: domainGUID1,
					},
					Spec: networkingv1alpha1.CFDomainSpec{
						Name: domainName1,
					},
				}
				Expect(k8sClient.Create(ctx, cfDomain1)).To(Succeed())

				cfDomain2 = &networkingv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name: domainGUID2,
					},
					Spec: networkingv1alpha1.CFDomainSpec{
						Name: domainName2,
					},
				}
				Expect(k8sClient.Create(ctx, cfDomain2)).To(Succeed())
			})

			AfterEach(func() {
				ctx := context.Background()
				k8sClient.Delete(ctx, cfDomain1)
				k8sClient.Delete(ctx, cfDomain2)
			})

			It("eventually returns a list of domainRecords for each CFDomain CR", func() {
				var domainRecords []DomainRecord
				Eventually(func() []DomainRecord {
					domainRecords, _ = domainRepo.FetchDomainList(testCtx, domainClient, domainListMessage)
					return domainRecords
				}, timeCheckThreshold*time.Second).Should(HaveLen(2), "returned records count should equal number of created CRs")

				var domain1, domain2 DomainRecord
				for _, domainRecord := range domainRecords {
					switch domainRecord.GUID {
					case cfDomain1.Name:
						domain1 = domainRecord
					case cfDomain2.Name:
						domain2 = domainRecord
					default:
						Fail(fmt.Sprintf("Unknown domainRecord: %v", domainRecord))
					}
				}

				Expect(domain1).NotTo(BeZero())
				Expect(domain2).NotTo(BeZero())

				By("returning a domainRecord in the list for one of the created CRs", func() {
					Expect(domain1.GUID).To(Equal(cfDomain1.Name))
					Expect(domain1.Name).To(Equal(cfDomain1.Spec.Name))

					createdAt, err := time.Parse(time.RFC3339, domain1.CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

					updatedAt, err := time.Parse(time.RFC3339, domain1.CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
				})

				By("returning a domainRecord in the list that matches another of the created CRs", func() {
					Expect(domain2.GUID).To(Equal(cfDomain2.Name))
					Expect(domain2.Name).To(Equal(cfDomain2.Spec.Name))

					createdAt, err := time.Parse(time.RFC3339, domain2.CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

					updatedAt, err := time.Parse(time.RFC3339, domain2.CreatedAt)
					Expect(err).NotTo(HaveOccurred())
					Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
				})
			})
		})

		When("no CFDomain exist", func() {
			It("returns an empty list and no error", func() {
				domainRecords, err := domainRepo.FetchDomainList(testCtx, domainClient, domainListMessage)
				Expect(err).ToNot(HaveOccurred())
				Expect(domainRecords).To(BeEmpty())
			})
		})

		When("multiple CFDomain exists and a filter is provided", func() {
			const (
				domainName1 = "my-domain-name-1"
				domainName2 = "my-domain-name-2"
			)
			var (
				domainGUID1 string
				domainGUID2 string

				cfDomain1 *networkingv1alpha1.CFDomain
				cfDomain2 *networkingv1alpha1.CFDomain
			)

			BeforeEach(func() {
				domainGUID1 = generateGUID()
				domainGUID2 = generateGUID()

				ctx := context.Background()
				domainListMessage = DomainListMessage{
					Names: []string{domainName1},
				}

				cfDomain1 = &networkingv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name: domainGUID1,
					},
					Spec: networkingv1alpha1.CFDomainSpec{
						Name: domainName1,
					},
				}
				Expect(k8sClient.Create(ctx, cfDomain1)).To(Succeed())

				cfDomain2 = &networkingv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name: domainGUID2,
					},
					Spec: networkingv1alpha1.CFDomainSpec{
						Name: domainName2,
					},
				}
				Expect(k8sClient.Create(ctx, cfDomain2)).To(Succeed())
			})

			AfterEach(func() {
				ctx := context.Background()
				k8sClient.Delete(ctx, cfDomain1)
				k8sClient.Delete(ctx, cfDomain2)
			})

			When("a single value is provided for a key", func() {
				It("eventually returns a list of domainRecords for each CFDomain CR that matches the key with value", func() {
					var domainRecords []DomainRecord
					Eventually(func() []DomainRecord {
						domainRecords, _ = domainRepo.FetchDomainList(testCtx, domainClient, domainListMessage)
						return domainRecords
					}, timeCheckThreshold*time.Second).Should(HaveLen(1), "returned records count should equal number of created CRs")

					var domain1 DomainRecord
					for _, domainRecord := range domainRecords {
						switch domainRecord.GUID {
						case cfDomain1.Name:
							domain1 = domainRecord
						default:
							Fail(fmt.Sprintf("Unknown domainRecord: %v", domainRecord))
						}
					}

					Expect(domain1).NotTo(BeZero())

					By("returning a domainRecord in the list for one of the created CRs", func() {
						Expect(domain1.GUID).To(Equal(cfDomain1.Name))
						Expect(domain1.Name).To(Equal(cfDomain1.Spec.Name))

						createdAt, err := time.Parse(time.RFC3339, domain1.CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

						updatedAt, err := time.Parse(time.RFC3339, domain1.CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
					})
				})
			})

			When("a multiple value is provided for a key", func() {
				BeforeEach(func() {
					domainListMessage = DomainListMessage{
						Names: []string{domainName1, domainName2},
					}
				})

				It("eventually returns a list of domainRecords for each CFDomain CR that matches the key with value", func() {
					var domainRecords []DomainRecord
					Eventually(func() []DomainRecord {
						domainRecords, _ = domainRepo.FetchDomainList(testCtx, domainClient, domainListMessage)
						return domainRecords
					}, timeCheckThreshold*time.Second).Should(HaveLen(2), "returned records count should equal number of created CRs")

					var domain1, domain2 DomainRecord
					for _, domainRecord := range domainRecords {
						switch domainRecord.GUID {
						case cfDomain1.Name:
							domain1 = domainRecord
						case cfDomain2.Name:
							domain2 = domainRecord
						default:
							Fail(fmt.Sprintf("Unknown domainRecord: %v", domainRecord))
						}
					}

					Expect(domain1).NotTo(BeZero())
					Expect(domain2).NotTo(BeZero())

					By("returning a domainRecord in the list for one of the created CRs", func() {
						Expect(domain1.GUID).To(Equal(cfDomain1.Name))
						Expect(domain1.Name).To(Equal(cfDomain1.Spec.Name))

						createdAt, err := time.Parse(time.RFC3339, domain1.CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

						updatedAt, err := time.Parse(time.RFC3339, domain1.CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
					})

					By("returning a domainRecord in the list that matches another of the created CRs", func() {
						Expect(domain2.GUID).To(Equal(cfDomain2.Name))
						Expect(domain2.Name).To(Equal(cfDomain2.Spec.Name))

						createdAt, err := time.Parse(time.RFC3339, domain2.CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

						updatedAt, err := time.Parse(time.RFC3339, domain2.CreatedAt)
						Expect(err).NotTo(HaveOccurred())
						Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
					})
				})
			})
		})
	})
})
