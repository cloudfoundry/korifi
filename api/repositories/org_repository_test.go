package repositories_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = FDescribe("OrgRepository", func() {
	var (
		ctx     context.Context
		orgRepo *repositories.OrgRepo
	)

	BeforeEach(func() {
		ctx = context.Background()
		orgRepo = repositories.NewOrgRepo(rootNamespace, k8sClient, userClientFactory, nsPerms, 2*time.Minute)
	})

	Describe("Create", func() {
		var (
			createErr error
			orgName   string
			org       repositories.OrgRecord
		)

		BeforeEach(func() {
			orgName = prefixedGUID("org-name")
		})

		JustBeforeEach(func() {
			org, createErr = orgRepo.CreateOrg(ctx, authInfo, repositories.CreateOrgMessage{
				Name: orgName,
			})
		})

		When("the user doesn't have the admin role", func() {
			It("fails when creating an org", func() {
				Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the user has the admin role", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			})

			It("creates a CFOrg resource in the root namespace", func() {
				Expect(createErr).NotTo(HaveOccurred())

				orgCR := new(korifiv1alpha1.CFOrg)
				Expect(k8sClient.Get(ctx, client.ObjectKey{Namespace: rootNamespace, Name: org.GUID}, orgCR)).To(Succeed())

				Expect(org.Name).To(Equal(orgName))
				Expect(org.GUID).To(HavePrefix("cf-org-"))
				Expect(org.CreatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
				Expect(org.UpdatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
			})

			When("the org isn't ready in the timeout", func() {
				BeforeEach(func() {
					disableControllers()
				})

				It("returns an error", func() {
					Expect(createErr).To(MatchError(ContainSubstring("cf org did not get Condition `Ready`: 'True'")))
				})
			})

			When("the client fails to create the org", func() {
				BeforeEach(func() {
					orgName = "this-string-has-illegal-characters-ц"
				})

				It("returns an error", func() {
					Expect(createErr).To(HaveOccurred())
				})
			})
		})
	})

	Describe("List", func() {
		var cfOrg1, cfOrg2, cfOrg3 *korifiv1alpha1.CFOrg

		BeforeEach(func() {
			ctx = context.Background()

			cfOrg1 = createOrgWithCleanup(ctx, prefixedGUID("org1"))
			createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg1.Name)
			cfOrg2 = createOrgWithCleanup(ctx, prefixedGUID("org2"))
			createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg2.Name)
			cfOrg3 = createOrgWithCleanup(ctx, prefixedGUID("org3"))
			createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg3.Name)
		})

		It("returns the 3 orgs", func() {
			orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{})
			Expect(err).NotTo(HaveOccurred())

			Expect(orgs).To(ContainElements(
				repositories.OrgRecord{
					Name:      cfOrg1.Spec.DisplayName,
					CreatedAt: cfOrg1.CreationTimestamp.Time,
					UpdatedAt: cfOrg1.CreationTimestamp.Time,
					GUID:      cfOrg1.Name,
				},
				repositories.OrgRecord{
					Name:      cfOrg2.Spec.DisplayName,
					CreatedAt: cfOrg2.CreationTimestamp.Time,
					UpdatedAt: cfOrg2.CreationTimestamp.Time,
					GUID:      cfOrg2.Name,
				},
				repositories.OrgRecord{
					Name:      cfOrg3.Spec.DisplayName,
					CreatedAt: cfOrg3.CreationTimestamp.Time,
					UpdatedAt: cfOrg3.CreationTimestamp.Time,
					GUID:      cfOrg3.Name,
				},
			))
		})

		When("the org is not ready", func() {
			BeforeEach(func() {
				meta.SetStatusCondition(&(cfOrg1.Status.Conditions), metav1.Condition{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "because",
					Message: "because",
				})
				Expect(k8sClient.Status().Update(ctx, cfOrg1)).To(Succeed())

				meta.SetStatusCondition(&(cfOrg2.Status.Conditions), metav1.Condition{
					Type:    "Ready",
					Status:  metav1.ConditionUnknown,
					Reason:  "because",
					Message: "because",
				})
				Expect(k8sClient.Status().Update(ctx, cfOrg2)).To(Succeed())
			})

			It("does not list it", func() {
				orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{})
				Expect(err).NotTo(HaveOccurred())

				Expect(orgs).NotTo(ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(cfOrg1.Name),
					}),
				))
				Expect(orgs).NotTo(ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(cfOrg2.Name),
					}),
				))
				Expect(orgs).To(ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(cfOrg3.Name),
					}),
				))
			})
		})

		When("we filter for names org1 and org3", func() {
			It("returns just those", func() {
				orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{Names: []string{cfOrg1.Spec.DisplayName, cfOrg3.Spec.DisplayName}})
				Expect(err).NotTo(HaveOccurred())

				Expect(orgs).To(ConsistOf(
					repositories.OrgRecord{
						Name:      cfOrg1.Spec.DisplayName,
						CreatedAt: cfOrg1.CreationTimestamp.Time,
						UpdatedAt: cfOrg1.CreationTimestamp.Time,
						GUID:      cfOrg1.Name,
					},
					repositories.OrgRecord{
						Name:      cfOrg3.Spec.DisplayName,
						CreatedAt: cfOrg3.CreationTimestamp.Time,
						UpdatedAt: cfOrg3.CreationTimestamp.Time,
						GUID:      cfOrg3.Name,
					},
				))
			})
		})

		When("we filter for guids org1 and org3", func() {
			It("returns just those", func() {
				orgs, err := orgRepo.ListOrgs(ctx, authInfo, repositories.ListOrgsMessage{GUIDs: []string{cfOrg1.Name, cfOrg2.Name}})
				Expect(err).NotTo(HaveOccurred())

				Expect(orgs).To(ConsistOf(
					repositories.OrgRecord{
						Name:      cfOrg1.Spec.DisplayName,
						CreatedAt: cfOrg1.CreationTimestamp.Time,
						UpdatedAt: cfOrg1.CreationTimestamp.Time,
						GUID:      cfOrg1.Name,
					},
					repositories.OrgRecord{
						Name:      cfOrg2.Spec.DisplayName,
						CreatedAt: cfOrg2.CreationTimestamp.Time,
						UpdatedAt: cfOrg2.CreationTimestamp.Time,
						GUID:      cfOrg2.Name,
					},
				))
			})
		})

		When("fetching authorized namespaces fails", func() {
			var listErr error

			BeforeEach(func() {
				_, listErr = orgRepo.ListOrgs(ctx, authorization.Info{}, repositories.ListOrgsMessage{Names: []string{cfOrg1.Spec.DisplayName, cfOrg3.Spec.DisplayName}})
			})

			It("returns the error", func() {
				Expect(listErr).To(MatchError(ContainSubstring("failed to get identity")))
			})
		})
	})

	Describe("Get", func() {
		var cfOrg *korifiv1alpha1.CFOrg

		BeforeEach(func() {
			cfOrg = createOrgWithCleanup(ctx, prefixedGUID("the-org"))
		})

		When("the user has a role binding in the org", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg.Name)
			})

			It("gets the org", func() {
				orgRecord, err := orgRepo.GetOrg(ctx, authInfo, cfOrg.Name)
				Expect(err).NotTo(HaveOccurred())
				Expect(orgRecord.Name).To(Equal(cfOrg.Spec.DisplayName))
			})
		})

		When("the org isn't found", func() {
			It("errors", func() {
				_, err := orgRepo.GetOrg(ctx, authInfo, "non-existent-org")
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})
	})

	Describe("Delete", func() {
		var cfOrg *korifiv1alpha1.CFOrg

		BeforeEach(func() {
			cfOrg = createOrgWithCleanup(ctx, prefixedGUID("org"))
		})

		When("the user has permission to delete orgs", func() {
			BeforeEach(func() {
				beforeCtx := context.Background()
				createRoleBinding(beforeCtx, userName, adminRole.Name, cfOrg.Namespace)
				// Controllers don't exist in env-test environments, we manually copy role bindings to child ns.
				createRoleBinding(beforeCtx, userName, adminRole.Name, cfOrg.Name)
			})

			When("on the happy path", func() {
				It("deletes the CF Org resource", func() {
					deletionId, err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
						GUID: cfOrg.Name,
					})
					Expect(err).NotTo(HaveOccurred())
					Expect(deletionId).To(Equal("org.delete~" + cfOrg.Name))

					Eventually(func(g Gomega) {
						err = k8sClient.Get(ctx, client.ObjectKey{Namespace: rootNamespace, Name: cfOrg.Name}, &korifiv1alpha1.CFOrg{})
						g.Expect(err).To(MatchError(ContainSubstring("not found")))
					}).Should(Succeed())
				})
			})

			When("the org doesn't exist", func() {
				It("errors", func() {
					_, err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
						GUID: "non-existent-org",
					})
					Expect(err).To(MatchError(ContainSubstring("not found")))
				})
			})
		})

		When("the user does not have permission to delete orgs", func() {
			It("errors with forbidden", func() {
				_, err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
					GUID: cfOrg.Name,
				})
				Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})

			When("the org doesn't exist", func() {
				It("errors with forbidden", func() {
					_, err := orgRepo.DeleteOrg(ctx, authInfo, repositories.DeleteOrgMessage{
						GUID: "non-existent-org",
					})
					Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
				})
			})
		})
	})
})
