package repositories_test

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	. "code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/controllers/apis/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("DomainRepository", func() {
	var (
		testCtx    context.Context
		domainRepo *DomainRepo
	)

	BeforeEach(func() {
		domainRepo = NewDomainRepo(userClientFactory, namespaceRetriever, rootNamespace)
		testCtx = context.Background()
	})

	Describe("GetDomain", func() {
		When("multiple CFDomain resources exist", func() {
			var (
				cfDomain1 *v1alpha1.CFDomain
				cfDomain2 *v1alpha1.CFDomain
			)

			BeforeEach(func() {
				beforeCtx := context.Background()

				cfDomain1 = &v1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "domain-id-1",
						Namespace: rootNamespace,
					},
					Spec: v1alpha1.CFDomainSpec{
						Name: "my-domain-1.com",
					},
				}
				Expect(k8sClient.Create(beforeCtx, cfDomain1)).To(Succeed())

				cfDomain2 = &v1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "domain-id-2",
						Namespace: rootNamespace,
					},
					Spec: v1alpha1.CFDomainSpec{
						Name: "my-domain-2.com",
					},
				}
				Expect(k8sClient.Create(beforeCtx, cfDomain2)).To(Succeed())
			})

			AfterEach(func() {
				afterCtx := context.Background()
				Expect(k8sClient.Delete(afterCtx, cfDomain1)).To(Succeed())
				Expect(k8sClient.Delete(afterCtx, cfDomain2)).To(Succeed())
			})

			It("fetches the CFDomain CR we're looking for", func() {
				domain, err := domainRepo.GetDomain(testCtx, authInfo, "domain-id-1")
				Expect(err).ToNot(HaveOccurred())

				Expect(domain.GUID).To(Equal("domain-id-1"))
				Expect(domain.Name).To(Equal("my-domain-1.com"))
			})
		})

		When("no CFDomain exists", func() {
			It("returns an error", func() {
				_, err := domainRepo.GetDomain(testCtx, authInfo, "non-existent-domain-guid")
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("ListDomains", func() {
		var domainListMessage ListDomainsMessage

		BeforeEach(func() {
			domainListMessage = ListDomainsMessage{}
		})

		When("the user has permission to list domains in the root namespace", func() {
			BeforeEach(func() {
				createRoleBinding(context.Background(), userName, rootNamespaceUserRole.Name, rootNamespace)
			})

			When("multiple CFDomains exist and no filter is provided", func() {
				const (
					domainName1 = "my-domain-name-1"
					domainName2 = "my-domain-name-2"
					domainName3 = "my-domain-name-3"
				)
				var (
					domainGUID1 string
					domainGUID2 string
					domainGUID3 string

					cfDomain1 *v1alpha1.CFDomain
					cfDomain2 *v1alpha1.CFDomain
					cfDomain3 *v1alpha1.CFDomain
				)

				BeforeEach(func() {
					domainGUID1 = generateGUID()
					domainGUID2 = generateGUID()
					domainGUID3 = generateGUID()

					cfDomain1 = &v1alpha1.CFDomain{
						ObjectMeta: metav1.ObjectMeta{
							Name:      domainGUID1,
							Namespace: rootNamespace,
						},
						Spec: v1alpha1.CFDomainSpec{
							Name: domainName1,
						},
					}
					Expect(
						k8sClient.Create(testCtx, cfDomain1),
					).To(Succeed())

					cfDomain2 = &v1alpha1.CFDomain{
						ObjectMeta: metav1.ObjectMeta{
							Name:      domainGUID2,
							Namespace: rootNamespace,
						},
						Spec: v1alpha1.CFDomainSpec{
							Name: domainName2,
						},
					}
					Expect(
						k8sClient.Create(testCtx, cfDomain2),
					).To(Succeed())

					cfDomain3 = &v1alpha1.CFDomain{
						ObjectMeta: metav1.ObjectMeta{
							Name:      domainGUID3,
							Namespace: rootNamespace,
						},
						Spec: v1alpha1.CFDomainSpec{
							Name: domainName3,
						},
					}
					time.Sleep(1 * time.Second)
					Expect(
						k8sClient.Create(testCtx, cfDomain3),
					).To(Succeed())
				})

				AfterEach(func() {
					Expect(
						k8sClient.Delete(context.Background(), cfDomain1),
					).To(Succeed())
					Expect(
						k8sClient.Delete(context.Background(), cfDomain2),
					).To(Succeed())
					Expect(
						k8sClient.Delete(context.Background(), cfDomain3),
					).To(Succeed())
				})

				It("returns an ordered list(oldest to newest) of domainRecords for each CFDomain CR", func() {
					domainRecords, err := domainRepo.ListDomains(testCtx, authInfo, domainListMessage)
					Expect(err).NotTo(HaveOccurred())

					Expect(domainRecords).To(ContainElements(
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(domainGUID1)}),
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(domainGUID2)}),
						MatchFields(IgnoreExtras, Fields{"GUID": Equal(domainGUID3)}),
					))

					for i := 0; i < len(domainRecords)-1; i++ {
						currentRecordCreatedAt, err := time.Parse(time.RFC3339, domainRecords[i].CreatedAt)
						Expect(err).NotTo(HaveOccurred())

						nextRecordCreatedAt, err := time.Parse(time.RFC3339, domainRecords[i+1].CreatedAt)
						Expect(err).NotTo(HaveOccurred())

						Expect(currentRecordCreatedAt).To(BeTemporally("<=", nextRecordCreatedAt))
					}
				})
			})

			When("no CFDomains exist", func() {
				It("returns an empty list and no error", func() {
					domainRecords, err := domainRepo.ListDomains(testCtx, authInfo, domainListMessage)
					Expect(err).ToNot(HaveOccurred())
					Expect(domainRecords).To(BeEmpty())
				})
			})

			When("multiple CFDomains exist and a filter is provided", func() {
				const (
					domainName1 = "my-domain-name-1"
					domainName2 = "my-domain-name-2"
				)
				var (
					domainGUID1 string
					domainGUID2 string

					cfDomain1 *v1alpha1.CFDomain
					cfDomain2 *v1alpha1.CFDomain
				)

				BeforeEach(func() {
					domainGUID1 = generateGUID()
					domainGUID2 = generateGUID()

					ctx := context.Background()
					domainListMessage = ListDomainsMessage{
						Names: []string{domainName1},
					}

					cfDomain1 = &v1alpha1.CFDomain{
						ObjectMeta: metav1.ObjectMeta{
							Name:      domainGUID1,
							Namespace: rootNamespace,
						},
						Spec: v1alpha1.CFDomainSpec{
							Name: domainName1,
						},
					}
					Expect(
						k8sClient.Create(ctx, cfDomain1),
					).To(Succeed())

					cfDomain2 = &v1alpha1.CFDomain{
						ObjectMeta: metav1.ObjectMeta{
							Name:      domainGUID2,
							Namespace: rootNamespace,
						},
						Spec: v1alpha1.CFDomainSpec{
							Name: domainName2,
						},
					}
					Expect(
						k8sClient.Create(ctx, cfDomain2),
					).To(Succeed())
				})

				AfterEach(func() {
					Expect(
						k8sClient.Delete(testCtx, cfDomain1),
					).To(Succeed())
					Expect(
						k8sClient.Delete(testCtx, cfDomain2),
					).To(Succeed())
				})

				When("a single value is provided for a key", func() {
					It("returns a list of domainRecords for each CFDomain CR that matches the key with value", func() {
						domainRecords, err := domainRepo.ListDomains(testCtx, authInfo, domainListMessage)
						Expect(err).NotTo(HaveOccurred())

						Expect(domainRecords).To(HaveLen(1))
						Expect(domainRecords[0].GUID).To(Equal(cfDomain1.Name))

						By("returning a domainRecord in the list for one of the created CRs", func() {
							Expect(domainRecords[0].GUID).To(Equal(cfDomain1.Name))
							Expect(domainRecords[0].Name).To(Equal(cfDomain1.Spec.Name))

							createdAt, err := time.Parse(time.RFC3339, domainRecords[0].CreatedAt)
							Expect(err).NotTo(HaveOccurred())
							Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

							updatedAt, err := time.Parse(time.RFC3339, domainRecords[0].CreatedAt)
							Expect(err).NotTo(HaveOccurred())
							Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))
						})
					})
				})

				When("multiple values are provided for a key", func() {
					BeforeEach(func() {
						domainListMessage = ListDomainsMessage{
							Names: []string{domainName1, domainName2},
						}
					})

					It("returns a list of domainRecords for each CFDomain CR that matches the key with value", func() {
						domainRecords, err := domainRepo.ListDomains(testCtx, authInfo, domainListMessage)
						Expect(err).NotTo(HaveOccurred())

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

		When("The user has no permissions to list domains and multiple domains exist", func() {
			const (
				domainName1 = "my-domain-name-1"
				domainName2 = "my-domain-name-2"
			)

			var (
				tempRootNamespace *corev1.Namespace

				domainGUID1 string
				domainGUID2 string

				cfDomain1 *v1alpha1.CFDomain
				cfDomain2 *v1alpha1.CFDomain
			)

			BeforeEach(func() {
				beforeCtx := context.Background()

				tempRootNamespace = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: generateGUID()}}
				Expect(
					k8sClient.Create(beforeCtx, tempRootNamespace),
				).To(Succeed())
				domainRepo = NewDomainRepo(userClientFactory, namespaceRetriever, tempRootNamespace.Name)
				domainGUID1 = generateGUID()
				domainGUID2 = generateGUID()

				domainListMessage = ListDomainsMessage{
					Names: []string{domainName1},
				}

				cfDomain1 = &v1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name:      domainGUID1,
						Namespace: tempRootNamespace.Name,
					},
					Spec: v1alpha1.CFDomainSpec{
						Name: domainName1,
					},
				}
				Expect(
					k8sClient.Create(beforeCtx, cfDomain1),
				).To(Succeed())

				cfDomain2 = &v1alpha1.CFDomain{
					ObjectMeta: metav1.ObjectMeta{
						Name:      domainGUID2,
						Namespace: tempRootNamespace.Name,
					},
					Spec: v1alpha1.CFDomainSpec{
						Name: domainName2,
					},
				}
				Expect(
					k8sClient.Create(beforeCtx, cfDomain2),
				).To(Succeed())
			})

			AfterEach(func() {
				Expect(
					k8sClient.Delete(testCtx, cfDomain1),
				).To(Succeed())
				Expect(
					k8sClient.Delete(testCtx, cfDomain2),
				).To(Succeed())
				Expect(
					k8sClient.Delete(testCtx, tempRootNamespace),
				).To(Succeed())
			})

			It("returns an empty list and no error", func() {
				domainRecords, err := domainRepo.ListDomains(testCtx, authInfo, domainListMessage)
				Expect(domainRecords).To(BeEmpty())
				Expect(err).ToNot(HaveOccurred())
			})
		})
	})

	Describe("GetDomainByName", func() {
		const (
			domainName = "fetchdomainbyname.test"
		)

		var (
			cfDomain   *v1alpha1.CFDomain
			cfDomain2  *v1alpha1.CFDomain
			domainGUID string
		)

		BeforeEach(func() {
			beforeCtx := context.Background()

			domainGUID = generateGUID()
			cfDomain = &v1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      domainGUID,
					Namespace: rootNamespace,
				},
				Spec: v1alpha1.CFDomainSpec{
					Name: domainName,
				},
			}
			Expect(
				k8sClient.Create(beforeCtx, cfDomain),
			).To(Succeed())

			cfDomain2 = &v1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      generateGUID(),
					Namespace: rootNamespace,
				},
				Spec: v1alpha1.CFDomainSpec{
					Name: "some-other-domain.com",
				},
			}
			Expect(
				k8sClient.Create(beforeCtx, cfDomain2),
			).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(context.Background(), cfDomain)).To(Succeed())
			Expect(k8sClient.Delete(context.Background(), cfDomain2)).To(Succeed())
		})

		When("One match exists for the provided name", func() {
			It("returns a domainRecord that matches the specified domain name, and no error", func() {
				domainRecord, err := domainRepo.GetDomainByName(context.Background(), authInfo, domainName)
				Expect(err).NotTo(HaveOccurred())
				Expect(domainRecord.GUID).To(Equal(domainGUID))
				Expect(domainRecord.Name).To(Equal(domainName))
			})
		})

		When("No matches exist for the provided name", func() {
			It("returns a domainRecord that matches the specified domain name, and no error", func() {
				_, err := domainRepo.GetDomainByName(context.Background(), authInfo, "i-dont-exist")
				Expect(err).To(BeAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})
})
