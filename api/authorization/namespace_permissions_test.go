package authorization_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apis/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var _ = Describe("Namespace Permissions", func() {
	var (
		ctx              context.Context
		nsPerms          *authorization.NamespacePermissions
		namespaces       map[string]bool
		getErr           error
		authInfo         authorization.Info
		identity         authorization.Identity
		identityProvider *fake.IdentityProvider

		rootNamespace        string
		org1NS, org2NS       string
		space1NS, space2NS   string
		nonCFNS              string
		userName             string
		roleName1, roleName2 string
	)

	createNamespace := func(name string, labels map[string]string) string {
		guid := generateGUID(name)
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   guid,
				Labels: labels,
			},
		})).To(Succeed())

		return guid
	}

	createClusterRole := func(name string) *rbacv1.ClusterRole {
		role := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}

		Expect(k8sClient.Create(ctx, role)).To(Succeed())

		return role
	}

	createRoleBindingForUser := func(user, roleName, namespace string) *rbacv1.RoleBinding {
		role := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", user, roleName),
				Namespace: namespace,
			},
			Subjects: []rbacv1.Subject{
				{
					Name: user,
					Kind: "User",
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     roleName,
			},
		}

		Expect(k8sClient.Create(ctx, role)).To(Succeed())

		return role
	}

	BeforeEach(func() {
		rootNamespace = generateGUID("root-ns")
		userName = generateGUID("alice")
		ctx = context.Background()

		authInfo = authorization.Info{
			Token: "alice-token",
		}
		identity = authorization.Identity{
			Name: userName,
			Kind: "User",
		}
		identityProvider = new(fake.IdentityProvider)
		identityProvider.GetIdentityReturns(identity, nil)

		nsPerms = authorization.NewNamespacePermissions(k8sClient, identityProvider, rootNamespace)

		nonCFNS = createNamespace("non-cf", nil)

		roleName1 = generateGUID("org-user-1")
		roleName2 = generateGUID("org-user-2")

		createClusterRole(roleName1)
		createClusterRole(roleName2)
		createRoleBindingForUser(userName, roleName2, nonCFNS)
	})

	AfterEach(func() {
		ctx = context.Background()
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nonCFNS}})).To(Succeed())
		Expect(k8sClient.Delete(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: roleName1}})).To(Succeed())
		Expect(k8sClient.Delete(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: roleName2}})).To(Succeed())
	})

	Describe("Get Authorized Org Namespaces", func() {
		BeforeEach(func() {
			org1NS = createNamespace("org1", map[string]string{rootNamespace + v1alpha2.LabelTreeDepthSuffix: "1"})
			org2NS = createNamespace("org2", map[string]string{rootNamespace + v1alpha2.LabelTreeDepthSuffix: "1"})

			createRoleBindingForUser(userName, roleName1, org1NS)
			createRoleBindingForUser(userName, roleName2, org1NS)
			createRoleBindingForUser("some-other-user", roleName1, org2NS)
		})

		JustBeforeEach(func() {
			namespaces, getErr = nsPerms.GetAuthorizedOrgNamespaces(ctx, authInfo)
		})

		AfterEach(func() {
			ctx = context.Background()
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: org1NS}})).To(Succeed())
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: org2NS}})).To(Succeed())
		})

		It("lists the namespaces with bindings for current user", func() {
			Expect(getErr).NotTo(HaveOccurred())
			Expect(namespaces).To(Equal(map[string]bool{org1NS: true}))
		})

		When("the id provider fails", func() {
			BeforeEach(func() {
				identityProvider.GetIdentityReturns(authorization.Identity{}, errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(getErr).To(MatchError(ContainSubstring("failed to get identity")))
			})
		})

		When("the user does not have a rolebinding associated with it", func() {
			BeforeEach(func() {
				identityProvider.GetIdentityReturns(authorization.Identity{
					Name: generateGUID("bob"),
					Kind: "User",
				}, nil)
			})

			It("returns an empty list", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(namespaces).To(BeEmpty())
			})
		})

		When("listing the rolebindings fails", func() {
			var cancelCtx context.CancelFunc

			BeforeEach(func() {
				ctx, cancelCtx = context.WithDeadline(ctx, time.Now().Add(-time.Minute))
			})

			AfterEach(func() {
				cancelCtx()
			})

			It("returns an error", func() {
				Expect(getErr).To(MatchError(ContainSubstring("failed to list rolebindings")))
			})
		})
	})

	Describe("Get Authorized Space Namespaces", func() {
		BeforeEach(func() {
			space1NS = createNamespace("space1", map[string]string{rootNamespace + v1alpha2.LabelTreeDepthSuffix: "2"})
			space2NS = createNamespace("space2", map[string]string{rootNamespace + v1alpha2.LabelTreeDepthSuffix: "2"})

			createRoleBindingForUser(userName, roleName1, space1NS)
			createRoleBindingForUser("some-other-user", roleName1, space2NS)
		})

		JustBeforeEach(func() {
			namespaces, getErr = nsPerms.GetAuthorizedSpaceNamespaces(ctx, authInfo)
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: space1NS}})).To(Succeed())
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: space2NS}})).To(Succeed())
		})

		It("lists the namespaces with bindings for current user", func() {
			Expect(getErr).NotTo(HaveOccurred())
			Expect(namespaces).To(Equal(map[string]bool{space1NS: true}))
		})

		When("the user does not have a rolebinding associated with it", func() {
			BeforeEach(func() {
				identityProvider.GetIdentityReturns(authorization.Identity{
					Name: generateGUID("bob"),
					Kind: "User",
				}, nil)
			})

			It("returns an empty list", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(namespaces).To(BeEmpty())
			})
		})
	})

	Describe("Authorized In", func() {
		BeforeEach(func() {
			org1NS = createNamespace("org1", map[string]string{rootNamespace + v1alpha2.LabelTreeDepthSuffix: "1"})
			org2NS = createNamespace("org2", map[string]string{rootNamespace + v1alpha2.LabelTreeDepthSuffix: "1"})

			createRoleBindingForUser(userName, roleName1, org1NS)
			createRoleBindingForUser("some-other-user", roleName1, org2NS)
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: org1NS}})).To(Succeed())
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: org2NS}})).To(Succeed())
		})

		When("the user has a rolebinding in the namespace", func() {
			It("returns true", func() {
				authorized, err := nsPerms.AuthorizedIn(ctx, identity, org1NS)
				Expect(err).NotTo(HaveOccurred())
				Expect(authorized).To(BeTrue())
			})
		})

		When("the user does not have a RoleBinding in the namespace", func() {
			It("returns false", func() {
				authorized, err := nsPerms.AuthorizedIn(ctx, identity, org2NS)
				Expect(err).NotTo(HaveOccurred())
				Expect(authorized).To(BeFalse())
			})
		})
	})
})

func generateGUID(prefix string) string {
	guid := uuid.NewString()
	return fmt.Sprintf("%s-%s", prefix, guid[:6])
}
