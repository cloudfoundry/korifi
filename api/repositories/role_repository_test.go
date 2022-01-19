package repositories_test

import (
	"context"
	"errors"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/config"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/fake"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("RoleRepository", func() {
	var (
		ctx                 context.Context
		rootNamespace       string
		roleCreateMessage   repositories.CreateRoleMessage
		roleRepo            *repositories.RoleRepo
		orgAnchor           *hnsv1alpha2.SubnamespaceAnchor
		createdRole         repositories.RoleRecord
		authorizedInChecker *fake.AuthorizedInChecker
		createErr           error
		clientFactory       repositories.UserK8sClientFactory
		orgManagerRole      *rbacv1.ClusterRole
		orgUserRole         *rbacv1.ClusterRole
		spaceDeveloperRole  *rbacv1.ClusterRole
	)

	BeforeEach(func() {
		rootNamespace = uuid.NewString()
		ctx = context.Background()
		authorizedInChecker = new(fake.AuthorizedInChecker)
		Expect(k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}})).To(Succeed())
		clientFactory = repositories.NewUnprivilegedClientFactory(k8sConfig)
		orgManagerRole = createOrgManagerClusterRole(ctx)
		orgUserRole = createOrgUserClusterRole(ctx)
		spaceDeveloperRole = createSpaceDeveloperClusterRole(ctx)
		roleRepo = repositories.NewRoleRepo(k8sClient, clientFactory, authorizedInChecker, map[string]config.Role{
			"space_developer":      {Name: spaceDeveloperRole.Name},
			"organization_manager": {Name: orgManagerRole.Name, Propagate: true},
			"organization_user":    {Name: orgUserRole.Name},
		})

		roleCreateMessage = repositories.CreateRoleMessage{}
		orgAnchor = createOrgAnchorAndNamespace(ctx, rootNamespace, uuid.NewString())
	})

	getTheRoleBinding := func(name, namespace string) rbacv1.RoleBinding {
		roleBinding := rbacv1.RoleBinding{}
		ExpectWithOffset(1, k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &roleBinding)).To(Succeed())

		return roleBinding
	}

	Describe("Create Org Role", func() {
		BeforeEach(func() {
			roleCreateMessage = repositories.CreateRoleMessage{
				GUID: uuid.NewString(),
				Type: "organization_manager",
				User: "myuser@example.com",
				Kind: rbacv1.UserKind,
				Org:  orgAnchor.Name,
			}
		})

		JustBeforeEach(func() {
			createdRole, createErr = roleRepo.CreateRole(ctx, authInfo, roleCreateMessage)
		})

		When("the user doesn't have permissions to create roles", func() {
			It("fails", func() {
				Expect(createErr).To(BeAssignableToTypeOf(repositories.ForbiddenError{}))
			})
		})

		When("the user is an admin", func() {
			var expectedName string

			BeforeEach(func() {
				// Sha256 sum of "organization_manager::myuser@example.com"
				expectedName = "cf-172b9594a1f617258057870643bce8476179a4078845cb4d9d44171d7a8b648b"
				adminClusterRole := createAdminClusterRole(ctx)
				createRoleBinding(ctx, userName, adminClusterRole.Name, orgAnchor.Name)
			})

			It("succeeds", func() {
				Expect(createErr).NotTo(HaveOccurred())
			})

			It("creates a role binding in the org namespace", func() {
				roleBinding := getTheRoleBinding(expectedName, orgAnchor.Name)

				Expect(roleBinding.Labels).To(HaveKeyWithValue(repositories.RoleGuidLabel, roleCreateMessage.GUID))
				Expect(roleBinding.RoleRef.Kind).To(Equal("ClusterRole"))
				Expect(roleBinding.RoleRef.Name).To(Equal(orgManagerRole.Name))
				Expect(roleBinding.Subjects).To(HaveLen(1))
				Expect(roleBinding.Subjects[0].Kind).To(Equal(rbacv1.UserKind))
				Expect(roleBinding.Subjects[0].Name).To(Equal("myuser@example.com"))
			})

			It("updated the create/updated timestamps", func() {
				Expect(createdRole.CreatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
				Expect(createdRole.UpdatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
				Expect(createdRole.CreatedAt).To(Equal(createdRole.UpdatedAt))
			})

			Describe("Role propagation", func() {
				When("the org role has propagation enabled", func() {
					BeforeEach(func() {
						roleCreateMessage.Type = "organization_manager"
					})

					It("enables the role binding propagation", func() {
						Expect(getTheRoleBinding(expectedName, orgAnchor.Name).Annotations).NotTo(HaveKey(HavePrefix(hnsv1alpha2.AnnotationPropagatePrefix)))
					})
				})

				When("the org role has propagation disabled", func() {
					BeforeEach(func() {
						roleCreateMessage.Type = "organization_user"
						// Sha256 sum of "organization_user::myuser@example.com"
						expectedName = "cf-2a6f4cbdd1777d57b5b7b2ee835785dafa68c147719c10948397cfc2ea7246a3"
					})

					It("enables the role binding propagation", func() {
						Expect(createErr).NotTo(HaveOccurred())
						Expect(getTheRoleBinding(expectedName, orgAnchor.Name).Annotations).To(HaveKeyWithValue(hnsv1alpha2.AnnotationNoneSelector, "true"))
					})
				})
			})

			When("using a service account identity", func() {
				BeforeEach(func() {
					roleCreateMessage.Kind = rbacv1.ServiceAccountKind
					roleCreateMessage.User = "my-service-account"
					// Sha256 sum of "organization_manager::my-service-account"
					expectedName = "cf-6af123f3cf60cbba6c34bfa5f13314151ba309a9d7a9a19464aa052c773542e0"
				})

				It("succeeds and uses a service account subject kind", func() {
					Expect(createErr).NotTo(HaveOccurred())

					roleBinding := getTheRoleBinding(expectedName, orgAnchor.Name)
					Expect(roleBinding.Subjects).To(HaveLen(1))
					Expect(roleBinding.Subjects[0].Name).To(Equal("my-service-account"))
					Expect(roleBinding.Subjects[0].Kind).To(Equal(rbacv1.ServiceAccountKind))
				})
			})

			When("the org does not exist", func() {
				BeforeEach(func() {
					roleCreateMessage.Org = "i-do-not-exist"
				})

				It("returns an error", func() {
					Expect(createErr).To(BeAssignableToTypeOf(repositories.ForbiddenError{}))
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
				It("returns an error", func() {
					anotherRoleCreateMessage := repositories.CreateRoleMessage{
						GUID: uuid.NewString(),
						Type: "organization_manager",
						User: "myuser@example.com",
						Kind: rbacv1.UserKind,
						Org:  roleCreateMessage.Org,
					}
					_, createErr = roleRepo.CreateRole(ctx, authInfo, anotherRoleCreateMessage)
					Expect(createErr).To(Equal(repositories.ErrorDuplicateRoleBinding))
				})
			})
		})
	})

	Describe("Create Space Role", func() {
		var (
			spaceAnchor  *hnsv1alpha2.SubnamespaceAnchor
			expectedName string
		)

		BeforeEach(func() {
			// Sha256 sum of "space_developer::myuser@example.com"
			expectedName = "cf-94662df3659074e12fbb2a05fbda554db8fd0bf2f59394874412ebb0dddf6ba4"
			authorizedInChecker.AuthorizedInReturns(true, nil)
			spaceAnchor = createSpaceAnchorAndNamespace(ctx, orgAnchor.Name, uuid.NewString())

			roleCreateMessage = repositories.CreateRoleMessage{
				GUID:  uuid.NewString(),
				Type:  "space_developer",
				User:  "myuser@example.com",
				Space: spaceAnchor.Name,
				Kind:  rbacv1.UserKind,
			}

			adminClusterRole := createAdminClusterRole(ctx)
			createRoleBinding(ctx, userName, adminClusterRole.Name, spaceAnchor.Name)
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Create(context.Background(), &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: orgAnchor.Name,
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
			roleBinding := getTheRoleBinding(expectedName, spaceAnchor.Name)

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
			Expect(org).To(Equal(orgAnchor.Name))
		})

		It("updated the create/updated timestamps", func() {
			Expect(createdRole.CreatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
			Expect(createdRole.UpdatedAt).To(BeTemporally("~", time.Now(), 2*time.Second))
			Expect(createdRole.CreatedAt).To(Equal(createdRole.UpdatedAt))
		})

		When("using service accounts", func() {
			BeforeEach(func() {
				roleCreateMessage.Kind = rbacv1.ServiceAccountKind
				roleCreateMessage.User = "my-service-account"
			})

			It("sends the service account kind to the authorized in checker", func() {
				_, identity, _ := authorizedInChecker.AuthorizedInArgsForCall(0)
				Expect(identity.Kind).To(Equal(rbacv1.ServiceAccountKind))
				Expect(identity.Name).To(Equal("my-service-account"))
			})
		})

		When("getting the parent org fails", func() {
			BeforeEach(func() {
				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: spaceAnchor.Name,
						Annotations: map[string]string{
							hnsv1alpha2.SubnamespaceOf: orgAnchor.Name,
						},
					},
				}
				nsCopy := namespace.DeepCopy()
				nsCopy.Annotations[hnsv1alpha2.SubnamespaceOf] = ""

				Expect(k8sClient.Patch(ctx, nsCopy, client.MergeFrom(namespace))).To(Succeed())
			})

			It("returns an error", func() {
				Expect(createErr).To(MatchError(ContainSubstring("does not have a parent")))
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
				Expect(k8serrors.IsNotFound(createErr)).To(BeTrue())
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
			It("returns an error", func() {
				anotherRoleCreateMessage := repositories.CreateRoleMessage{
					GUID:  uuid.NewString(),
					Type:  "space_developer",
					User:  "myuser@example.com",
					Kind:  rbacv1.UserKind,
					Space: roleCreateMessage.Space,
				}
				_, createErr = roleRepo.CreateRole(ctx, authInfo, anotherRoleCreateMessage)
				Expect(createErr).To(Equal(repositories.ErrorDuplicateRoleBinding))
			})
		})

		When("the user does not have a role in the parent organization", func() {
			BeforeEach(func() {
				authorizedInChecker.AuthorizedInReturns(false, nil)
			})

			It("returns an error", func() {
				Expect(createErr).To(Equal(repositories.ErrorMissingRoleBindingInParentOrg))
			})
		})
	})
})
