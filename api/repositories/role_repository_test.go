package repositories_test

import (
	"context"
	"errors"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/config"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/fake"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("RoleRepository", func() {
	var (
		ctx                 context.Context
		rootNamespace       string
		roleCreateMessage   repositories.RoleCreateMessage
		roleRepo            *repositories.RoleRepo
		orgAnchor           *hnsv1alpha2.SubnamespaceAnchor
		createdRole         repositories.RoleRecord
		authorizedInChecker *fake.AuthorizedInChecker
		createErr           error
	)

	BeforeEach(func() {
		rootNamespace = uuid.NewString()
		ctx = context.Background()
		authorizedInChecker = new(fake.AuthorizedInChecker)
		Expect(k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rootNamespace}})).To(Succeed())
		roleRepo = repositories.NewRoleRepo(k8sClient, authorizedInChecker, map[string]config.Role{
			"space_developer":      {Name: "cf-space-dev-role"},
			"organization_manager": {Name: "cf-org-mgr-role", Propagate: true},
			"organization_user":    {Name: "cf-org-user-role"},
		})

		roleCreateMessage = repositories.RoleCreateMessage{}
		orgAnchor = createOrgAnchorAndNamespace(ctx, rootNamespace, uuid.NewString())
	})

	getTheRoleBinding := func(namespace string) rbacv1.RoleBinding {
		roleBindingList := rbacv1.RoleBindingList{}
		ExpectWithOffset(1, k8sClient.List(ctx, &roleBindingList, client.InNamespace(namespace))).To(Succeed())
		ExpectWithOffset(1, roleBindingList.Items).To(HaveLen(1))

		return roleBindingList.Items[0]
	}

	Describe("Create Org Role", func() {
		BeforeEach(func() {
			roleCreateMessage = repositories.RoleCreateMessage{
				GUID: uuid.NewString(),
				Type: "organization_manager",
				User: "myuser@example.com",
				Kind: rbacv1.UserKind,
				Org:  orgAnchor.Name,
			}
		})

		JustBeforeEach(func() {
			createdRole, createErr = roleRepo.CreateRole(ctx, roleCreateMessage)
		})

		It("succeeds", func() {
			Expect(createErr).NotTo(HaveOccurred())
		})

		It("creates a role binding in the org namespace", func() {
			roleBinding := getTheRoleBinding(orgAnchor.Name)

			// Sha256 sum of "organization_manager::myuser@example.com"
			Expect(roleBinding.Name).To(Equal("cf-172b9594a1f617258057870643bce8476179a4078845cb4d9d44171d7a8b648b"))
			Expect(roleBinding.Labels).To(HaveKeyWithValue(repositories.RoleGuidLabel, roleCreateMessage.GUID))
			Expect(roleBinding.RoleRef.Kind).To(Equal("ClusterRole"))
			Expect(roleBinding.RoleRef.Name).To(Equal("cf-org-mgr-role"))
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
					Expect(getTheRoleBinding(orgAnchor.Name).Annotations).NotTo(HaveKey(HavePrefix(hnsv1alpha2.AnnotationPropagatePrefix)))
				})
			})

			When("the org role has propagation disabled", func() {
				BeforeEach(func() {
					roleCreateMessage.Type = "organization_user"
				})

				It("enables the role binding propagation", func() {
					Expect(getTheRoleBinding(orgAnchor.Name).Annotations).To(HaveKeyWithValue(hnsv1alpha2.AnnotationNoneSelector, "true"))
				})
			})
		})

		When("using a service account identity", func() {
			BeforeEach(func() {
				roleCreateMessage.Kind = rbacv1.ServiceAccountKind
				roleCreateMessage.User = "my-service-account"
			})

			It("succeeds and uses a service account subject kind", func() {
				Expect(createErr).NotTo(HaveOccurred())

				roleBinding := getTheRoleBinding(orgAnchor.Name)
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
				anotherRoleCreateMessage := repositories.RoleCreateMessage{
					GUID: uuid.NewString(),
					Type: "organization_manager",
					User: "myuser@example.com",
					Kind: rbacv1.UserKind,
					Org:  roleCreateMessage.Org,
				}
				_, createErr = roleRepo.CreateRole(ctx, anotherRoleCreateMessage)
				Expect(createErr).To(Equal(repositories.ErrorDuplicateRoleBinding))
			})
		})
	})

	Describe("Create Space Role", func() {
		var spaceAnchor *hnsv1alpha2.SubnamespaceAnchor

		BeforeEach(func() {
			authorizedInChecker.AuthorizedInReturns(true, nil)
			spaceAnchor = createSpaceAnchorAndNamespace(ctx, orgAnchor.Name, uuid.NewString())

			roleCreateMessage = repositories.RoleCreateMessage{
				GUID:  uuid.NewString(),
				Type:  "space_developer",
				User:  "myuser@example.com",
				Space: spaceAnchor.Name,
				Kind:  rbacv1.UserKind,
			}
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

			createdRole, createErr = roleRepo.CreateRole(ctx, roleCreateMessage)
		})

		It("succeeds", func() {
			Expect(createErr).NotTo(HaveOccurred())
		})

		It("creates a role binding in the space namespace", func() {
			roleBinding := getTheRoleBinding(spaceAnchor.Name)

			// Sha256 sum of "space_developer::myuser@example.com"
			Expect(roleBinding.Name).To(Equal("cf-94662df3659074e12fbb2a05fbda554db8fd0bf2f59394874412ebb0dddf6ba4"))
			Expect(roleBinding.Labels).To(HaveKeyWithValue(repositories.RoleGuidLabel, roleCreateMessage.GUID))
			Expect(roleBinding.RoleRef.Kind).To(Equal("ClusterRole"))
			Expect(roleBinding.RoleRef.Name).To(Equal("cf-space-dev-role"))
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
				anotherRoleCreateMessage := repositories.RoleCreateMessage{
					GUID:  uuid.NewString(),
					Type:  "space_developer",
					User:  "myuser@example.com",
					Kind:  rbacv1.UserKind,
					Space: roleCreateMessage.Space,
				}
				_, createErr = roleRepo.CreateRole(ctx, anotherRoleCreateMessage)
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
