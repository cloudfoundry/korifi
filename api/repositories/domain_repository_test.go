package repositories_test

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("DomainRepository", func() {
	var (
		testCtx    context.Context
		domainRepo *repositories.DomainRepo
	)

	BeforeEach(func() {
		testCtx = context.Background()
		domainRepo = NewDomainRepo(k8sClient)
	})

	Describe("FetchDomain", func() {
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
						Name: "my-domain-1.com",
					},
				}
				Expect(k8sClient.Create(beforeCtx, cfDomain1)).To(Succeed())

				cfDomain2 = &networkingv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name: "domain-id-2",
					},
					Spec: networkingv1alpha1.CFDomainSpec{
						Name: "my-domain-2.com",
					},
				}
				Expect(k8sClient.Create(beforeCtx, cfDomain2)).To(Succeed())
			})

			It("fetches the CFDomain CR we're looking for", func() {
				domain, err := domainRepo.FetchDomain(testCtx, authInfo, "domain-id-1")
				Expect(err).ToNot(HaveOccurred())

				Expect(domain.GUID).To(Equal("domain-id-1"))
				Expect(domain.Name).To(Equal("my-domain-1.com"))
			})

			AfterEach(func() {
				afterCtx := context.Background()
				Expect(k8sClient.Delete(afterCtx, cfDomain1)).To(Succeed())
				Expect(k8sClient.Delete(afterCtx, cfDomain2)).To(Succeed())
			})
		})

		When("no CFDomain exists", func() {
			It("returns an error", func() {
				_, err := domainRepo.FetchDomain(testCtx, authInfo, "non-existent-domain-guid")
				Expect(err).To(MatchError("Resource not found or permission denied."))
			})
		})
	})

	Describe("FetchDomainList", func() {
		var domainListMessage DomainListMessage

		BeforeEach(func() {
			domainListMessage = DomainListMessage{}
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

				cfDomain1 = &networkingv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name: domainGUID1,
					},
					Spec: networkingv1alpha1.CFDomainSpec{
						Name: domainName1,
					},
				}
				Expect(k8sClient.Create(testCtx, cfDomain1)).To(Succeed())

				cfDomain2 = &networkingv1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name: domainGUID2,
					},
					Spec: networkingv1alpha1.CFDomainSpec{
						Name: domainName2,
					},
				}
				Expect(k8sClient.Create(testCtx, cfDomain2)).To(Succeed())
			})

			AfterEach(func() {
				Expect(k8sClient.Delete(testCtx, cfDomain1)).To(Succeed())
				Expect(k8sClient.Delete(testCtx, cfDomain2)).To(Succeed())
			})

			It("eventually returns a list of domainRecords for each CFDomain CR", func() {
				var domainRecords []DomainRecord
				Eventually(func() []DomainRecord {
					domainRecords, _ = domainRepo.FetchDomainList(testCtx, authInfo, domainListMessage)
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
				domainRecords, err := domainRepo.FetchDomainList(testCtx, authInfo, domainListMessage)
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
				Expect(k8sClient.Delete(testCtx, cfDomain1)).To(Succeed())
				Expect(k8sClient.Delete(testCtx, cfDomain2)).To(Succeed())
			})

			When("a single value is provided for a key", func() {
				It("eventually returns a list of domainRecords for each CFDomain CR that matches the key with value", func() {
					var domainRecords []DomainRecord
					Eventually(func() []DomainRecord {
						domainRecords, _ = domainRepo.FetchDomainList(testCtx, authInfo, domainListMessage)
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
						domainRecords, _ = domainRepo.FetchDomainList(testCtx, authInfo, domainListMessage)
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

	Describe("FetchDomainByName", func() {
		const (
			domainName = "fetchdomainbyname.test"
		)

		var (
			cfDomain   *networkingv1alpha1.CFDomain
			domainGUID string
		)

		BeforeEach(func() {
			beforeCtx := context.Background()

			domainGUID = generateGUID()
			cfDomain = &networkingv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name: domainGUID,
				},
				Spec: networkingv1alpha1.CFDomainSpec{
					Name: domainName,
				},
			}
			Expect(
				k8sClient.Create(beforeCtx, cfDomain),
			).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(context.Background(), cfDomain)
			})

			cfDomain2 := &networkingv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name: generateGUID(),
				},
				Spec: networkingv1alpha1.CFDomainSpec{
					Name: "some-other-domain.com",
				},
			}
			Expect(
				k8sClient.Create(beforeCtx, cfDomain2),
			).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(context.Background(), cfDomain2)
			})
		})

		When("One match exists for the provided name", func() {
			It("returns a domainRecord that matches the specified domain name, and no error", func() {
				domainRecord, err := domainRepo.FetchDomainByName(context.Background(), authInfo, domainName)
				Expect(err).NotTo(HaveOccurred())
				Expect(domainRecord.GUID).To(Equal(domainGUID))
				Expect(domainRecord.Name).To(Equal(domainName))
			})
		})

		When("No matches exist for the provided name", func() {
			It("returns a domainRecord that matches the specified domain name, and no error", func() {
				_, err := domainRepo.FetchDomainByName(context.Background(), authInfo, "i-dont-exist")
				Expect(err).To(MatchError(NotFoundError{ResourceType: "Domain"}))
			})
		})
	})
})
