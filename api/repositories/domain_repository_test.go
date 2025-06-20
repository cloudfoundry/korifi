package repositories_test

import (
	"context"
	"time"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"code.cloudfoundry.org/korifi/api/authorization/testhelpers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("DomainRepository", func() {
	var (
		domainRepo *repositories.DomainRepo
		cfDomain   *korifiv1alpha1.CFDomain
		domainGUID string
		domainName string
	)

	BeforeEach(func() {
		domainName = "my-domain.com"
		domainGUID = uuid.NewString()
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

		domainRepo = repositories.NewDomainRepo(rootNSKlient, rootNamespace)
	})

	AfterEach(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, cfDomain))).To(Succeed())
	})

	Describe("GetDomain", func() {
		var (
			searchGUID string
			domain     repositories.DomainRecord
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
			domainCreate  repositories.CreateDomainMessage
			createdDomain repositories.DomainRecord
			createErr     error
		)

		BeforeEach(func() {
			domainCreate = repositories.CreateDomainMessage{
				Name: "my.domain",
				Metadata: repositories.Metadata{
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
				Expect(createdDomainGUID).To(matchers.BeValidUUID())
				Expect(createdDomain.Name).To(Equal("my.domain"))
				Expect(createdDomain.Labels).To(HaveKeyWithValue("foo", "bar"))
				Expect(createdDomain.Annotations).To(HaveKeyWithValue("bar", "baz"))

				Expect(createdDomain.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(createdDomain.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))

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
			updatePayload repositories.UpdateDomainMessage
			updatedDomain repositories.DomainRecord
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

			updatePayload = repositories.UpdateDomainMessage{
				GUID: cfDomain.Name,
				MetadataPatch: repositories.MetadataPatch{
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
			domainListMessage repositories.ListDomainsMessage
			listResult        repositories.ListResult[repositories.DomainRecord]
		)

		BeforeEach(func() {
			domainListMessage = repositories.ListDomainsMessage{}
			createRoleBinding(context.Background(), userName, rootNamespaceUserRole.Name, rootNamespace)
		})

		JustBeforeEach(func() {
			var err error
			listResult, err = domainRepo.ListDomains(ctx, authInfo, domainListMessage)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns domain records", func() {
			Expect(listResult.Records).To(ContainElements(
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(domainGUID)}),
			))
		})

		Describe("list options", func() {
			var fakeKlient *fake.Klient

			BeforeEach(func() {
				fakeKlient = new(fake.Klient)
				domainRepo = repositories.NewDomainRepo(fakeKlient, rootNamespace)
			})

			Describe("parameters to list options", func() {
				BeforeEach(func() {
					domainListMessage = repositories.ListDomainsMessage{
						Names:   []string{"n1", "n2"},
						OrderBy: "created_at",
						Pagination: repositories.Pagination{
							Page:    3,
							PerPage: 4,
						},
					}
				})

				It("translates parameters to klient list options", func() {
					Expect(fakeKlient.ListCallCount()).To(Equal(1))
					_, _, listOptions := fakeKlient.ListArgsForCall(0)
					Expect(listOptions).To(ConsistOf(
						repositories.WithLabelIn(korifiv1alpha1.CFEncodedDomainNameLabelKey, tools.EncodeValuesToSha224("n1", "n2")),
						repositories.WithOrdering("created_at"),
						repositories.WithPaging(repositories.Pagination{PerPage: 4, Page: 3}),
					))
				})
			})
		})

		When("the user has no permission to list domains in the root namespace", func() {
			BeforeEach(func() {
				userName = uuid.NewString()
				cert, key := testhelpers.ObtainClientCert(testEnv, userName)
				authInfo.CertData = testhelpers.JoinCertAndKey(cert, key)
			})

			It("returns an empty list and no error because the user has no permissions", func() {
				Expect(listResult.Records).To(BeEmpty())
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

	Describe("GetDeletedAt", func() {
		var (
			deletionTime *time.Time
			getErr       error
		)

		JustBeforeEach(func() {
			deletionTime, getErr = domainRepo.GetDeletedAt(ctx, authInfo, cfDomain.Name)
		})

		It("returns nil", func() {
			Expect(getErr).NotTo(HaveOccurred())
			Expect(deletionTime).To(BeNil())
		})

		When("the domain is being deleted", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, k8sClient, cfDomain, func() {
					cfDomain.Finalizers = append(cfDomain.Finalizers, "foo")
				})).To(Succeed())

				Expect(k8sClient.Delete(ctx, cfDomain)).To(Succeed())
			})

			It("returns the deletion time", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(deletionTime).To(PointTo(BeTemporally("~", time.Now(), time.Minute)))
			})
		})

		When("the domain isn't found", func() {
			BeforeEach(func() {
				Expect(k8sClient.Delete(ctx, cfDomain)).To(Succeed())
			})

			It("errors", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})
})
