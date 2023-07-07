package repositories_test

import (
	"context"
	"errors"
	"time"

	"code.cloudfoundry.org/korifi/api/config"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("RoleRepository", func() {
	var (
		roleCreateMessage   repositories.CreateRoleMessage
		roleRepo            *repositories.RoleRepo
		cfOrg               *korifiv1alpha1.CFOrg
		createdRole         repositories.RoleRecord
		authorizedInChecker *fake.AuthorizedInChecker
		createErr           error
	)

	BeforeEach(func() {
		authorizedInChecker = new(fake.AuthorizedInChecker)
		roleMappings := map[string]config.Role{
			"space_developer":      {Name: spaceDeveloperRole.Name, Level: config.SpaceRole},
			"organization_manager": {Name: orgManagerRole.Name, Level: config.OrgRole, Propagate: true},
			"organization_user":    {Name: orgUserRole.Name, Level: config.OrgRole},
			"cf_user":              {Name: rootNamespaceUserRole.Name},
			"admin":                {Name: adminRole.Name, Propagate: true},
		}
		orgRepo := repositories.NewOrgRepo(rootNamespace, k8sClient, userClientFactory, nsPerms, time.Millisecond*2000)
		spaceRepo := repositories.NewSpaceRepo(namespaceRetriever, orgRepo, userClientFactory, nsPerms, time.Millisecond*2000)
		roleRepo = repositories.NewRoleRepo(
			userClientFactory,
			spaceRepo,
			authorizedInChecker,
			nsPerms,
			rootNamespace,
			roleMappings,
			namespaceRetriever,
		)

		roleCreateMessage = repositories.CreateRoleMessage{}
		cfOrg = createOrgWithCleanup(ctx, uuid.NewString())
	})

	getTheRoleBinding := func(name, namespace string) rbacv1.RoleBinding {
		GinkgoHelper()

		roleBinding := rbacv1.RoleBinding{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &roleBinding)).To(Succeed())

		return roleBinding
	}

	Describe("Create Org Role", func() {
		BeforeEach(func() {
			roleCreateMessage = repositories.CreateRoleMessage{
				GUID: uuid.NewString(),
				Type: "organization_manager",
				User: "myuser@example.com",
				Kind: rbacv1.UserKind,
				Org:  cfOrg.Name,
			}
		})

		JustBeforeEach(func() {
			createdRole, createErr = roleRepo.CreateRole(ctx, authInfo, roleCreateMessage)
		})

		When("the user doesn't have permissions to create roles", func() {
			It("fails", func() {
				Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the user is an admin", func() {
			var (
				expectedName       string
				cfUserExpectedName string
			)

			BeforeEach(func() {
				// Sha256 sum of "organization_manager::myuser@example.com"
				expectedName = "cf-172b9594a1f617258057870643bce8476179a4078845cb4d9d44171d7a8b648b"
				// Sha256 sum of "cf_user::myuser@example.com"
				cfUserExpectedName = "cf-156eb9a28b4143e61a5b43fb7e7a6b8de98495aa4b5da4ba871dc4eaa4c35433"
				createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
				createRoleBinding(ctx, userName, adminRole.Name, cfOrg.Name)
			})

			It("succeeds", func() {
				Expect(createErr).NotTo(HaveOccurred())
			})

			It("creates a role binding in the org namespace", func() {
				roleBinding := getTheRoleBinding(expectedName, cfOrg.Name)

				Expect(roleBinding.Labels).To(HaveKeyWithValue(repositories.RoleGuidLabel, roleCreateMessage.GUID))
				Expect(roleBinding.RoleRef.Kind).To(Equal("ClusterRole"))
				Expect(roleBinding.RoleRef.Name).To(Equal(orgManagerRole.Name))
				Expect(roleBinding.Subjects).To(HaveLen(1))
				Expect(roleBinding.Subjects[0].Kind).To(Equal(rbacv1.UserKind))
				Expect(roleBinding.Subjects[0].Name).To(Equal("myuser@example.com"))
			})

			It("creates a role binding for cf_user in the root namespace", func() {
				roleBinding := getTheRoleBinding(cfUserExpectedName, rootNamespace)

				Expect(roleBinding.RoleRef.Kind).To(Equal("ClusterRole"))
				Expect(roleBinding.RoleRef.Name).To(Equal(rootNamespaceUserRole.Name))
				Expect(roleBinding.Subjects).To(HaveLen(1))
				Expect(roleBinding.Subjects[0].Kind).To(Equal(rbacv1.UserKind))
				Expect(roleBinding.Subjects[0].Name).To(Equal("myuser@example.com"))
			})

			It("updated the create/updated timestamps", func() {
				Expect(createdRole.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
				Expect(createdRole.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
				Expect(createdRole.UpdatedAt).To(PointTo(Equal(createdRole.CreatedAt)))
			})

			Describe("Role propagation", func() {
				When("the org role has propagation enabled", func() {
					BeforeEach(func() {
						roleCreateMessage.Type = "organization_manager"
					})

					It("enables the role binding propagation, but not for cf_user", func() {
						Expect(getTheRoleBinding(expectedName, cfOrg.Name).Annotations).To(HaveKeyWithValue(korifiv1alpha1.PropagateRoleBindingAnnotation, "true"))
						Expect(getTheRoleBinding(cfUserExpectedName, rootNamespace).Annotations).To(HaveKeyWithValue(korifiv1alpha1.PropagateRoleBindingAnnotation, "false"))
					})
				})

				When("the org role has propagation deactivated", func() {
					BeforeEach(func() {
						roleCreateMessage.Type = "organization_user"
						// Sha256 sum of "organization_user::myuser@example.com"
						expectedName = "cf-2a6f4cbdd1777d57b5b7b2ee835785dafa68c147719c10948397cfc2ea7246a3"
					})

					It("deactivates the role binding propagation", func() {
						Expect(createErr).NotTo(HaveOccurred())
						Expect(getTheRoleBinding(expectedName, cfOrg.Name).Annotations).To(HaveKeyWithValue(korifiv1alpha1.PropagateRoleBindingAnnotation, "false"))
						Expect(getTheRoleBinding(cfUserExpectedName, rootNamespace).Annotations).To(HaveKeyWithValue(korifiv1alpha1.PropagateRoleBindingAnnotation, "false"))
					})
				})
			})

			When("using a service account identity", func() {
				BeforeEach(func() {
					roleCreateMessage.Kind = rbacv1.ServiceAccountKind
					roleCreateMessage.User = "my-service-account"
					roleCreateMessage.ServiceAccountNamespace = "my-namespace"
					// Sha256 sum of "organization_manager::my-namespace/my-service-account"
					expectedName = "cf-aff6351a3949461e600a128524e2849af0afb4d3d5bd94e36e2189df3e4130b8"
				})

				It("succeeds and uses a service account subject kind", func() {
					Expect(createErr).NotTo(HaveOccurred())

					roleBinding := getTheRoleBinding(expectedName, cfOrg.Name)
					Expect(roleBinding.Subjects).To(HaveLen(1))
					Expect(roleBinding.Subjects[0].Name).To(Equal("my-service-account"))
					Expect(roleBinding.Subjects[0].Kind).To(Equal(rbacv1.ServiceAccountKind))
					Expect(roleBinding.Subjects[0].Namespace).To(Equal("my-namespace"))
				})
			})

			When("the org does not exist", func() {
				BeforeEach(func() {
					roleCreateMessage.Org = "i-do-not-exist"
				})

				It("returns an error", func() {
					Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
				})
			})

			When("the role type is invalid", func() {
				BeforeEach(func() {
					roleCreateMessage.Type = "i-am-invalid"
				})

				It("returns an error", func() {
					Expect(createErr).To(MatchError(ContainSubstring("invalid role type")))
				})
			})

			When("the user is already bound to that role", func() {
				It("returns an unprocessable entity error", func() {
					anotherRoleCreateMessage := repositories.CreateRoleMessage{
						GUID: uuid.NewString(),
						Type: "organization_manager",
						User: "myuser@example.com",
						Kind: rbacv1.UserKind,
						Org:  roleCreateMessage.Org,
					}
					_, createErr = roleRepo.CreateRole(ctx, authInfo, anotherRoleCreateMessage)
					var apiErr apierrors.UnprocessableEntityError
					Expect(errors.As(createErr, &apiErr)).To(BeTrue())
					// Note: the cf cli expects this specific format and ignores the error if it matches it.
					Expect(apiErr.Detail()).To(Equal("User 'myuser@example.com' already has 'organization_manager' role"))
				})
			})
		})
	})

	Describe("Create Space Role", func() {
		var (
			cfSpace      *korifiv1alpha1.CFSpace
			expectedName string
		)

		BeforeEach(func() {
			// Sha256 sum of "space_developer::myuser@example.com"
			expectedName = "cf-94662df3659074e12fbb2a05fbda554db8fd0bf2f59394874412ebb0dddf6ba4"
			authorizedInChecker.AuthorizedInReturns(true, nil)
			cfSpace = createSpaceWithCleanup(ctx, cfOrg.Name, uuid.NewString())

			roleCreateMessage = repositories.CreateRoleMessage{
				GUID:  uuid.NewString(),
				Type:  "space_developer",
				User:  "myuser@example.com",
				Space: cfSpace.Name,
				Kind:  rbacv1.UserKind,
			}

			createRoleBinding(ctx, userName, adminRole.Name, rootNamespace)
			createRoleBinding(ctx, userName, adminRole.Name, cfOrg.Name)
			createRoleBinding(ctx, userName, adminRole.Name, cfSpace.Name)
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Create(context.Background(), &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: cfOrg.Name,
				},
				Subjects: []rbacv1.Subject{
					{
						Kind: roleCreateMessage.Kind,
						Name: roleCreateMessage.User,
					},
				},
				RoleRef: rbacv1.RoleRef{
					Kind: "ClusterRole",
					Name: "org_user",
				},
			})).To(Succeed())

			createdRole, createErr = roleRepo.CreateRole(ctx, authInfo, roleCreateMessage)
		})

		It("succeeds", func() {
			Expect(createErr).NotTo(HaveOccurred())
		})

		It("creates a role binding in the space namespace", func() {
			roleBinding := getTheRoleBinding(expectedName, cfSpace.Name)

			Expect(roleBinding.Labels).To(HaveKeyWithValue(repositories.RoleGuidLabel, roleCreateMessage.GUID))
			Expect(roleBinding.RoleRef.Kind).To(Equal("ClusterRole"))
			Expect(roleBinding.RoleRef.Name).To(Equal(spaceDeveloperRole.Name))
			Expect(roleBinding.Subjects).To(HaveLen(1))
			Expect(roleBinding.Subjects[0].Kind).To(Equal(rbacv1.UserKind))
			Expect(roleBinding.Subjects[0].Name).To(Equal("myuser@example.com"))
		})

		It("verifies that the user has a role in the parent org", func() {
			Expect(authorizedInChecker.AuthorizedInCallCount()).To(Equal(1))
			_, userIdentity, org := authorizedInChecker.AuthorizedInArgsForCall(0)
			Expect(userIdentity.Name).To(Equal("myuser@example.com"))
			Expect(userIdentity.Kind).To(Equal(rbacv1.UserKind))
			Expect(org).To(Equal(cfOrg.Name))
		})

		It("updated the create/updated timestamps", func() {
			Expect(createdRole.CreatedAt).To(BeTemporally("~", time.Now(), timeCheckThreshold))
			Expect(createdRole.UpdatedAt).To(PointTo(BeTemporally("~", time.Now(), timeCheckThreshold)))
			Expect(createdRole.UpdatedAt).To(PointTo(Equal(createdRole.CreatedAt)))
		})

		When("using service accounts", func() {
			BeforeEach(func() {
				// Sha256 sum of "space_developer::my-namespace/my-service-account"
				expectedName = "cf-253970950188359abff344b5976af3fd888c9c10ef2972603ec7eb1ef5e82296"
				roleCreateMessage.Kind = rbacv1.ServiceAccountKind
				roleCreateMessage.User = "my-service-account"
				roleCreateMessage.ServiceAccountNamespace = "my-namespace"
			})

			It("sends the service account kind to the authorized in checker", func() {
				_, identity, _ := authorizedInChecker.AuthorizedInArgsForCall(0)
				Expect(identity.Kind).To(Equal(rbacv1.ServiceAccountKind))
				Expect(identity.Name).To(Equal("system:serviceaccount:my-namespace:my-service-account"))
			})

			It("creates a role binding in the space namespace", func() {
				roleBinding := getTheRoleBinding(expectedName, cfSpace.Name)

				Expect(roleBinding.Labels).To(HaveKeyWithValue(repositories.RoleGuidLabel, roleCreateMessage.GUID))
				Expect(roleBinding.RoleRef.Kind).To(Equal("ClusterRole"))
				Expect(roleBinding.RoleRef.Name).To(Equal(spaceDeveloperRole.Name))
				Expect(roleBinding.Subjects).To(HaveLen(1))
				Expect(roleBinding.Subjects[0].Kind).To(Equal(rbacv1.ServiceAccountKind))
				Expect(roleBinding.Subjects[0].Name).To(Equal("my-service-account"))
				Expect(roleBinding.Subjects[0].Namespace).To(Equal("my-namespace"))
			})
		})

		When("checking an org role exists fails", func() {
			BeforeEach(func() {
				authorizedInChecker.AuthorizedInReturns(false, errors.New("boom!"))
			})

			It("returns an error", func() {
				Expect(createErr).To(MatchError(ContainSubstring("failed to check for role in parent org")))
			})
		})

		When("the space does not exist", func() {
			BeforeEach(func() {
				roleCreateMessage.Space = "i-do-not-exist"
			})

			It("returns an error", func() {
				Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
			})
		})

		When("the space is forbidden", func() {
			BeforeEach(func() {
				anotherOrg := createOrgWithCleanup(ctx, uuid.NewString())
				cfSpace = createSpaceWithCleanup(ctx, anotherOrg.Name, uuid.NewString())
				roleCreateMessage.Space = cfSpace.Name
			})

			It("returns an error", func() {
				Expect(createErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
			})
		})

		When("the role type is invalid", func() {
			BeforeEach(func() {
				roleCreateMessage.Type = "i-am-invalid"
			})

			It("returns an error", func() {
				Expect(createErr).To(MatchError(ContainSubstring("invalid role type")))
			})
		})

		When("the user is already bound to that role", func() {
			It("returns an unprocessable entity error", func() {
				anotherRoleCreateMessage := repositories.CreateRoleMessage{
					GUID:  uuid.NewString(),
					Type:  "space_developer",
					User:  "myuser@example.com",
					Kind:  rbacv1.UserKind,
					Space: roleCreateMessage.Space,
				}
				_, createErr = roleRepo.CreateRole(ctx, authInfo, anotherRoleCreateMessage)
				Expect(createErr).To(SatisfyAll(
					BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}),
					MatchError(ContainSubstring("already exists")),
				))
			})
		})

		When("the user does not have a role in the parent organization", func() {
			BeforeEach(func() {
				authorizedInChecker.AuthorizedInReturns(false, nil)
			})

			It("returns an unprocessable entity error", func() {
				Expect(createErr).To(SatisfyAll(
					BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}),
					MatchError(ContainSubstring("no RoleBinding found")),
				))
			})
		})
	})

	Describe("list roles", func() {
		var (
			otherOrg            *korifiv1alpha1.CFOrg
			cfSpace, otherSpace *korifiv1alpha1.CFSpace
			roles               []repositories.RoleRecord
			listErr             error
		)

		BeforeEach(func() {
			otherOrg = createOrgWithCleanup(ctx, uuid.NewString())
			cfSpace = createSpaceWithCleanup(ctx, cfOrg.Name, uuid.NewString())
			otherSpace = createSpaceWithCleanup(ctx, cfOrg.Name, uuid.NewString())
			createRoleBinding(ctx, "my-user", orgUserRole.Name, cfOrg.Name, repositories.RoleGuidLabel, "1")
			createRoleBinding(ctx, "my-user", spaceDeveloperRole.Name, cfSpace.Name, repositories.RoleGuidLabel, "2")
			createRoleBinding(ctx, "my-user", spaceDeveloperRole.Name, otherSpace.Name, repositories.RoleGuidLabel, "3")
			createRoleBinding(ctx, "my-user", orgUserRole.Name, otherOrg.Name, repositories.RoleGuidLabel, "4")
		})

		JustBeforeEach(func() {
			roles, listErr = roleRepo.ListRoles(ctx, authInfo)
		})

		It("returns an empty list when user has no permissions to list roles", func() {
			Expect(listErr).NotTo(HaveOccurred())
			Expect(roles).To(BeEmpty())
		})

		When("the user has permission to list roles in some namespaces", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, orgUserRole.Name, cfOrg.Name, repositories.RoleGuidLabel, "5")
				createRoleBinding(ctx, userName, spaceDeveloperRole.Name, cfSpace.Name, repositories.RoleGuidLabel, "6")
			})

			It("returns the bindings in cfOrg and cfSpace only (for system user and my-user)", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(roles).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"GUID":  Equal("2"),
						"Kind":  Equal("User"),
						"User":  Equal("my-user"),
						"Type":  Equal("space_developer"),
						"Space": Equal(cfSpace.Name),
						"Org":   BeEmpty(),
					}),
					MatchFields(IgnoreExtras, Fields{
						"GUID":  Equal("1"),
						"Kind":  Equal("User"),
						"User":  Equal("my-user"),
						"Type":  Equal("organization_user"),
						"Space": BeEmpty(),
						"Org":   Equal(cfOrg.Name),
					}),
					MatchFields(IgnoreExtras, Fields{
						"GUID":  Equal("6"),
						"Kind":  Equal("User"),
						"User":  Equal(userName),
						"Type":  Equal("space_developer"),
						"Space": Equal(cfSpace.Name),
						"Org":   BeEmpty(),
					}),
					MatchFields(IgnoreExtras, Fields{
						"GUID":  Equal("5"),
						"Kind":  Equal("User"),
						"User":  Equal(userName),
						"Type":  Equal("organization_user"),
						"Space": BeEmpty(),
						"Org":   Equal(cfOrg.Name),
					}),
				))
			})

			When("there are propagated role bindings", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, "my-user", orgManagerRole.Name, cfSpace.Name, korifiv1alpha1.PropagatedFromLabel, "foo")
				})

				It("ignores them", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(roles).To(HaveLen(4))
				})
			})

			When("there are non-cf role bindings", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, "my-user", "some-role", cfSpace.Name)
				})

				It("ignores them", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(roles).To(HaveLen(4))
				})
			})

			When("there are root namespace permissions", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, userName, orgManagerRole.Name, rootNamespace)
				})

				It("ignores them", func() {
					Expect(listErr).NotTo(HaveOccurred())
					Expect(roles).To(HaveLen(4))
				})
			})
		})
	})

	Describe("delete role", func() {
		var (
			roleGUID  string
			deleteMsg repositories.DeleteRoleMessage
			deleteErr error
		)

		BeforeEach(func() {
			roleGUID = uuid.NewString()

			deleteMsg = repositories.DeleteRoleMessage{
				GUID: roleGUID,
				Org:  cfOrg.Name,
			}
		})

		JustBeforeEach(func() {
			createRoleBinding(ctx, "bob", orgManagerRole.Name, cfOrg.Name, repositories.RoleGuidLabel, roleGUID)

			deleteErr = roleRepo.DeleteRole(ctx, authInfo, deleteMsg)
		})

		When("the user doesn't have permissions to delete roles", func() {
			It("fails", func() {
				Expect(deleteErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.ForbiddenError{}))
			})
		})

		When("the user is allowed to delete roles", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, cfOrg.Name)
			})

			It("deletes the role binding", func() {
				Expect(deleteErr).NotTo(HaveOccurred())
				roleBindings := &rbacv1.RoleBindingList{}
				Expect(k8sClient.List(ctx, roleBindings, client.InNamespace(cfOrg.Name), client.MatchingLabels{
					repositories.RoleGuidLabel: roleGUID,
				})).To(Succeed())
				Expect(roleBindings.Items).To(BeEmpty())
			})

			When("there is no role with that guid", func() {
				BeforeEach(func() {
					roleGUID = "i-do-not-exist"
				})

				It("returns an error", func() {
					Expect(deleteErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})

			When("deleting space role", func() {
				BeforeEach(func() {
					deleteMsg.Org = ""
					deleteMsg.Space = cfOrg.Name
				})

				It("deletes the role binding", func() {
					Expect(deleteErr).NotTo(HaveOccurred())
					roleBindings := &rbacv1.RoleBindingList{}
					Expect(k8sClient.List(ctx, roleBindings, client.InNamespace(cfOrg.Name), client.MatchingLabels{
						repositories.RoleGuidLabel: roleGUID,
					})).To(Succeed())
					Expect(roleBindings.Items).To(BeEmpty())
				})
			})

			When("there are multiple role bindings with the specified guid", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, "bob", orgManagerRole.Name, cfOrg.Name, repositories.RoleGuidLabel, roleGUID)
				})

				It("returns an error", func() {
					Expect(deleteErr).To(MatchError(ContainSubstring("multiple role bindings")))
				})
			})
		})
	})

	Describe("get role", func() {
		var (
			guid        string
			roleBinding rbacv1.RoleBinding

			roleRecord repositories.RoleRecord
			getErr     error
		)

		BeforeEach(func() {
			guid = uuid.NewString()
			roleBinding = createRoleBinding(ctx, "bob", orgManagerRole.Name, cfOrg.Name, repositories.RoleGuidLabel, guid)
		})

		JustBeforeEach(func() {
			roleRecord, getErr = roleRepo.GetRole(ctx, authInfo, guid)
		})

		When("the user doesn't have role permissions", func() {
			It("fails", func() {
				Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})

		When("the user has permissions in the org", func() {
			BeforeEach(func() {
				createRoleBinding(ctx, userName, adminRole.Name, cfOrg.Name)
			})

			It("gets the role", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(roleRecord).To(Equal(repositories.RoleRecord{
					GUID:      guid,
					CreatedAt: roleBinding.CreationTimestamp.Time,
					UpdatedAt: tools.PtrTo(roleBinding.CreationTimestamp.Time),
					Type:      "organization_manager",
					Space:     "",
					Org:       cfOrg.Name,
					User:      "bob",
					Kind:      "User",
				}))
			})

			When("the role does not exist", func() {
				BeforeEach(func() {
					guid = "i-do-not-exist"
				})

				It("returns a not found error", func() {
					Expect(getErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})

			When("there are multiple role bindings with the same guid", func() {
				BeforeEach(func() {
					createRoleBinding(ctx, "bob", orgManagerRole.Name, cfOrg.Name, repositories.RoleGuidLabel, guid)
				})

				It("returns an error", func() {
					Expect(getErr).To(MatchError(ContainSubstring("multiple role bindings")))
				})
			})
		})
	})
})
