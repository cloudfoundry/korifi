package repositories_test

import (
	"context"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"code.cloudfoundry.org/korifi/api/authorization/testhelpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("DomainRepository", func() {
	var (
		domainRepo *DomainRepo
		cfDomain   *korifiv1alpha1.CFDomain
		domainGUID string
		domainName string
	)

	BeforeEach(func() {
		domainName = "my-domain.com"
		domainGUID = generateGUID()
		cfDomain = &korifiv1alpha1.CFDomain{
			ObjectMeta: metav1.ObjectMeta{
				Name:      domainGUID,
				Namespace: rootNamespace,
			},
			Spec: korifiv1alpha1.CFDomainSpec{
				Name: domainName,
			},
		}
		Expect(k8sClient.Create(ctx, cfDomain)).To(Succeed())

		domainRepo = NewDomainRepo(userClientFactory, namespaceRetriever, rootNamespace)
	})

	AfterEach(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cfDomain))).To(Succeed())
	})

	Describe("GetDomain", func() {
		var (
			searchGUID string
			domain     DomainRecord
			getErr     error
		)

		BeforeEach(func() {
			searchGUID = domainGUID
		})

		JustBeforeEach(func() {
			domain, getErr = domainRepo.GetDomain(ctx, authInfo, searchGUID)
		})

		It("fetches the CFDomain we're looking for", func() {
			Expect(getErr).NotTo(HaveOccurred())

			Expect(domain.GUID).To(Equal(domainGUID))
			Expect(domain.Name).To(Equal("my-domain.com"))
		})

		When("no CFDomain exists", func() {
			BeforeEach(func() {
				searchGUID = "i-dont-exist"
			})

			It("returns an error", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("CreateDomain", func() {
		var (
			domainCreate  CreateDomainMessage
			createdDomain DomainRecord
			createErr     error
		)

		BeforeEach(func() {
			domainCreate = CreateDomainMessage{
				Name: "my.domain",
				Metadata: Metadata{
					Labels: map[string]string{
						"foo": "bar",
					},
					Annotations: map[string]string{
						"bar": "baz",
					},
				},
			}
		})

		JustBeforeEach(func() {
			createdDomain, createErr = domainRepo.CreateDomain(ctx, authInfo, domainCreate)
		})

		It("fails because the user is not a CF admin", func() {
			Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a CFAdmin", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			})

			It("creates a domain record", func() {
				Expect(createErr).NotTo(HaveOccurred())

				createdDomainGUID := createdDomain.GUID
				Expect(createdDomainGUID).NotTo(BeEmpty())
				Expect(createdDomain.Name).To(Equal("my.domain"))
				Expect(createdDomain.Labels).To(HaveKeyWithValue("foo", "bar"))
				Expect(createdDomain.Annotations).To(HaveKeyWithValue("bar", "baz"))

				createdAt, err := time.Parse(time.RFC3339, createdDomain.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(createdAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

				updatedAt, err := time.Parse(time.RFC3339, createdDomain.CreatedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold*time.Second))

				domainNSName := types.NamespacedName{Name: createdDomainGUID, Namespace: rootNamespace}
				createdCFDomain := new(korifiv1alpha1.CFDomain)
				Expect(k8sClient.Get(ctx, domainNSName, createdCFDomain)).To(Succeed())

				Expect(createdCFDomain.Name).To(Equal(createdDomainGUID))
				Expect(createdCFDomain.Namespace).To(Equal(rootNamespace))
				Expect(createdCFDomain.Spec.Name).To(Equal("my.domain"))
				Expect(createdCFDomain.Labels).To(HaveKeyWithValue("foo", "bar"))
				Expect(createdCFDomain.Annotations).To(HaveKeyWithValue("bar", "baz"))
			})
		})
	})

	Describe("UpdateDomain", func() {
		var (
			updatePayload UpdateDomainMessage
			updatedDomain DomainRecord
			updateErr     error
		)

		BeforeEach(func() {
			Expect(k8s.Patch(ctx, k8sClient, cfDomain, func() {
				cfDomain.Labels = map[string]string{
					"foo": "bar",
				}
				cfDomain.Annotations = map[string]string{
					"baz": "bat",
				}
			})).To(Succeed())

			updatePayload = UpdateDomainMessage{
				GUID: cfDomain.Name,
				MetadataPatch: MetadataPatch{
					Labels: map[string]*string{
						"foo": tools.PtrTo("new-foo"),
					},
					Annotations: map[string]*string{
						"baz": tools.PtrTo("new-baz"),
					},
				},
			}
		})

		JustBeforeEach(func() {
			updatedDomain, updateErr = domainRepo.UpdateDomain(ctx, authInfo, updatePayload)
		})

		It("fails because the user is not a CF admin", func() {
			Expect(updateErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is a CFAdmin", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			})

			It("updates the domain metadata", func() {
				Expect(updateErr).NotTo(HaveOccurred())

				Expect(updatedDomain.Labels).To(HaveKeyWithValue("foo", "new-foo"))
				Expect(updatedDomain.Annotations).To(HaveKeyWithValue("baz", "new-baz"))
			})

			It("updates the domain metadata in kubernetes", func() {
				domainNSName := types.NamespacedName{Name: cfDomain.Name, Namespace: rootNamespace}
				updatedCFDomain := new(korifiv1alpha1.CFDomain)
				Expect(k8sClient.Get(ctx, domainNSName, updatedCFDomain)).To(Succeed())

				Expect(updatedDomain.Labels).To(HaveKeyWithValue("foo", "new-foo"))
				Expect(updatedDomain.Annotations).To(HaveKeyWithValue("baz", "new-baz"))
			})
		})
	})

	Describe("ListDomains", func() {
		var (
			domainListMessage ListDomainsMessage
			domainRecords     []DomainRecord
			listErr           error
		)

		BeforeEach(func() {
			domainListMessage = ListDomainsMessage{}
		})

		JustBeforeEach(func() {
			domainRecords, listErr = domainRepo.ListDomains(ctx, authInfo, domainListMessage)
		})

		BeforeEach(func() {
			createRoleBinding(context.Background(), userName, rootNamespaceUserRole.Name, rootNamespace)
		})

		var (
			domainGUID1 string
			cfDomain1   *korifiv1alpha1.CFDomain
		)

		BeforeEach(func() {
			domainGUID1 = generateGUID()

			cfDomain1 = &korifiv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      domainGUID1,
					Namespace: rootNamespace,
				},
				Spec: korifiv1alpha1.CFDomainSpec{
					Name: "domain-1",
				},
			}
			Expect(k8sClient.Create(ctx, cfDomain1)).To(Succeed())
		})

		AfterEach(func() {
			Expect(client.IgnoreNotFound(k8sClient.Delete(context.Background(), cfDomain1))).To(Succeed())
		})

		It("returns an ordered list(oldest to newest) of domainRecords for each CFDomain CR", func() {
			Expect(listErr).NotTo(HaveOccurred())

			Expect(domainRecords).To(ContainElements(
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(domainGUID)}),
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(domainGUID1)}),
			))

			firstDomainCreatedAt, err := time.Parse(time.RFC3339, domainRecords[0].CreatedAt)
			Expect(err).NotTo(HaveOccurred())
			secondDomainCreatedAt, err := time.Parse(time.RFC3339, domainRecords[1].CreatedAt)
			Expect(err).NotTo(HaveOccurred())
			Expect(firstDomainCreatedAt).To(BeTemporally("<=", secondDomainCreatedAt))
		})

		When("no CFDomains exist", func() {
			BeforeEach(func() {
				Expect(k8sClient.Delete(ctx, cfDomain)).To(Succeed())
				Expect(k8sClient.Delete(ctx, cfDomain1)).To(Succeed())
			})

			It("returns an empty list and no error", func() {
				Expect(listErr).ToNot(HaveOccurred())
				Expect(domainRecords).To(BeEmpty())
			})
		})

		When("a filter is provided", func() {
			BeforeEach(func() {
				domainListMessage = ListDomainsMessage{
					Names: []string{domainName},
				}
			})

			It("returns a list of domainRecords matching the filter", func() {
				Expect(listErr).NotTo(HaveOccurred())

				Expect(domainRecords).To(HaveLen(1))
				Expect(domainRecords[0].GUID).To(Equal(cfDomain.Name))
				Expect(domainRecords[0].Name).To(Equal(cfDomain.Spec.Name))
			})
		})

		When("the user has no permission to list domains in the root namespace", func() {
			BeforeEach(func() {
				userName = generateGUID()
				cert, key := testhelpers.ObtainClientCert(testEnv, userName)
				authInfo.CertData = testhelpers.JoinCertAndKey(cert, key)
			})

			It("returns an empty list and no error because the user has no permissions", func() {
				Expect(listErr).ToNot(HaveOccurred())
				Expect(domainRecords).To(BeEmpty())
			})
		})
	})

	Describe("GetDomainByName", func() {
		var (
			searchName  string
			foundDomain DomainRecord
			getErr      error
		)

		BeforeEach(func() {
			searchName = domainName
		})

		JustBeforeEach(func() {
			foundDomain, getErr = domainRepo.GetDomainByName(context.Background(), authInfo, searchName)
		})

		It("returns a domainRecord that matches the specified domain name, and no error", func() {
			Expect(getErr).NotTo(HaveOccurred())
			Expect(foundDomain.GUID).To(Equal(domainGUID))
			Expect(foundDomain.Name).To(Equal(domainName))
		})

		When("No matches exist for the provided name", func() {
			BeforeEach(func() {
				searchName = "fubar"
			})

			It("returns not found err", func() {
				Expect(getErr).To(BeAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("Delete Domain", func() {
		var (
			deleteGUID string
			deleteErr  error
		)

		BeforeEach(func() {
			deleteGUID = domainGUID
		})

		JustBeforeEach(func() {
			deleteErr = domainRepo.DeleteDomain(ctx, authInfo, deleteGUID)
		})

		It("returns a forbidden error", func() {
			Expect(deleteErr).To(BeAssignableToTypeOf(apierrors.ForbiddenError{}))
		})

		When("the user is permitted to delete domains", func() {
			BeforeEach(func() {
				createRoleBinding(context.Background(), userName, adminRole.Name, rootNamespace)
			})

			It("deletes the domain", func() {
				Expect(deleteErr).NotTo(HaveOccurred())

				Eventually(func(g Gomega) {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(cfDomain), &korifiv1alpha1.CFDomain{})
					g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})
})
