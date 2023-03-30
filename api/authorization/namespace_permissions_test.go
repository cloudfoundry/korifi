package authorization_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Namespace Permissions", func() {
	var (
		ctx                    context.Context
		nsPerms                *authorization.NamespacePermissions
		namespaces             map[string]bool
		getErr                 error
		authInfo               authorization.Info
		userIdentity           authorization.Identity
		serviceAccountIdentity authorization.Identity
		identityProvider       *fake.IdentityProvider

		space1NS, space2NS   string
		org1NS, org2NS       string
		nonCFNS              string
		userName             string
		serviceAccountName   string
		serviceAccountNS     string
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

	createRoleBindingForSubject := func(subject rbacv1.Subject, roleName, namespace string) *rbacv1.RoleBinding {
		role := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", subject.Name, roleName),
				Namespace: namespace,
			},
			Subjects: []rbacv1.Subject{subject},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     roleName,
			},
		}

		Expect(k8sClient.Create(ctx, role)).To(Succeed())

		return role
	}

	createRoleBindingForUser := func(user, roleName, namespace string) *rbacv1.RoleBinding {
		return createRoleBindingForSubject(rbacv1.Subject{Name: user, Kind: "User"}, roleName, namespace)
	}

	createRoleBindingForServiceAccount := func(serviceAccountName, serviceAccountNS, roleName, namespace string) *rbacv1.RoleBinding {
		return createRoleBindingForSubject(rbacv1.Subject{Name: serviceAccountName, Namespace: serviceAccountNS, Kind: "ServiceAccount"}, roleName, namespace)
	}

	BeforeEach(func() {
		userName = generateGUID("alice")
		serviceAccountName = generateGUID("service-account")
		serviceAccountNS = generateGUID("service-account-namespace")
		ctx = context.Background()

		authInfo = authorization.Info{
			Token: "the-auth-token",
		}
		userIdentity = authorization.Identity{
			Name: userName,
			Kind: "User",
		}
		serviceAccountIdentity = authorization.Identity{
			Name: fmt.Sprintf("system:serviceaccount:%s:%s", serviceAccountNS, serviceAccountName),
			Kind: "ServiceAccount",
		}
		identityProvider = new(fake.IdentityProvider)

		nsPerms = authorization.NewNamespacePermissions(k8sClient, identityProvider)

		nonCFNS = createNamespace("non-cf", nil)

		roleName1 = generateGUID("org-user-1")
		roleName2 = generateGUID("org-user-2")

		createClusterRole(roleName1)
		createClusterRole(roleName2)
		createRoleBindingForUser(userName, roleName2, nonCFNS)
		createRoleBindingForServiceAccount(serviceAccountName, serviceAccountNS, roleName2, nonCFNS)
	})

	AfterEach(func() {
		ctx = context.Background()
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nonCFNS}})).To(Succeed())
		Expect(k8sClient.Delete(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: roleName1}})).To(Succeed())
		Expect(k8sClient.Delete(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: roleName2}})).To(Succeed())
	})

	Describe("Get Authorized Org Namespaces", func() {
		BeforeEach(func() {
			org1NS = createNamespace("org1", map[string]string{korifiv1alpha1.OrgNameKey: "org1"})
			org2NS = createNamespace("org2", map[string]string{korifiv1alpha1.OrgNameKey: "org2"})
		})

		AfterEach(func() {
			ctx = context.Background()
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: org1NS}})).To(Succeed())
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: org2NS}})).To(Succeed())
		})

		JustBeforeEach(func() {
			namespaces, getErr = nsPerms.GetAuthorizedOrgNamespaces(ctx, authInfo)
		})

		When("a user is authenticated", func() {
			BeforeEach(func() {
				identityProvider.GetIdentityReturns(userIdentity, nil)
				createRoleBindingForUser(userName, roleName1, org1NS)
				createRoleBindingForUser(userName, roleName2, org1NS)
				createRoleBindingForUser("some-other-user", roleName1, org2NS)
			})

			It("lists the namespaces with bindings for current user", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(namespaces).To(Equal(map[string]bool{org1NS: true}))
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

		When("a service account is authenticated", func() {
			BeforeEach(func() {
				identityProvider.GetIdentityReturns(serviceAccountIdentity, nil)
				createRoleBindingForServiceAccount(serviceAccountName, serviceAccountNS, roleName1, org1NS)
				createRoleBindingForServiceAccount(serviceAccountName, serviceAccountNS, roleName2, org1NS)
				createRoleBindingForServiceAccount("some-other-service-account", "some-other-namespace", roleName1, org2NS)
				createRoleBindingForServiceAccount(serviceAccountName, "some-other-namespace", roleName2, org2NS)
			})

			It("lists the namespaces with bindings for current service account", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(namespaces).To(Equal(map[string]bool{org1NS: true}))
			})

			When("the service account does not have a rolebinding associated with it", func() {
				BeforeEach(func() {
					identityProvider.GetIdentityReturns(authorization.Identity{
						Name: fmt.Sprintf("system:serviceaccount:some-ns:%s", generateGUID("bob")),
						Kind: "ServiceAccount",
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

			When("the identity has an invalid service account name", func() {
				BeforeEach(func() {
					serviceAccountIdentity.Name = "name-without-prefix"
					identityProvider.GetIdentityReturns(serviceAccountIdentity, nil)
				})

				It("returns an error", func() {
					Expect(getErr).To(MatchError(ContainSubstring("system:serviceaccount:")))
				})
			})
		})

		When("the id provider fails", func() {
			BeforeEach(func() {
				identityProvider.GetIdentityReturns(authorization.Identity{}, errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(getErr).To(MatchError(ContainSubstring("failed to get identity")))
			})
		})
	})

	Describe("Get Authorized Space Namespaces", func() {
		BeforeEach(func() {
			space1NS = createNamespace("space1", map[string]string{korifiv1alpha1.SpaceNameKey: "space1"})
			space2NS = createNamespace("space2", map[string]string{korifiv1alpha1.SpaceNameKey: "space2"})
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: space1NS}})).To(Succeed())
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: space2NS}})).To(Succeed())
		})

		JustBeforeEach(func() {
			namespaces, getErr = nsPerms.GetAuthorizedSpaceNamespaces(ctx, authInfo)
		})

		When("a user is authenticated", func() {
			BeforeEach(func() {
				identityProvider.GetIdentityReturns(userIdentity, nil)
				createRoleBindingForUser(userName, roleName1, space1NS)
				createRoleBindingForUser("some-other-user", roleName1, space2NS)
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

		When("a service account is authenticated", func() {
			BeforeEach(func() {
				identityProvider.GetIdentityReturns(serviceAccountIdentity, nil)
				createRoleBindingForServiceAccount(serviceAccountName, serviceAccountNS, roleName1, space1NS)
				createRoleBindingForServiceAccount("some-other-service-account", serviceAccountNS, roleName1, space2NS)
				createRoleBindingForServiceAccount(serviceAccountName, "another-ns", roleName2, space2NS)
			})

			It("lists the namespaces with bindings for current service account", func() {
				Expect(getErr).NotTo(HaveOccurred())
				Expect(namespaces).To(Equal(map[string]bool{space1NS: true}))
			})

			When("the user does not have a rolebinding associated with it", func() {
				BeforeEach(func() {
					identityProvider.GetIdentityReturns(authorization.Identity{
						Name: fmt.Sprintf("system:serviceaccount:some-ns:%s", generateGUID("bob")),
						Kind: "ServiceAccount",
					}, nil)
				})

				It("returns an empty list", func() {
					Expect(getErr).NotTo(HaveOccurred())
					Expect(namespaces).To(BeEmpty())
				})
			})

			When("the identity has an invalid service account name", func() {
				BeforeEach(func() {
					serviceAccountIdentity.Name = "name-without-prefix"
					identityProvider.GetIdentityReturns(serviceAccountIdentity, nil)
				})

				It("returns an error", func() {
					Expect(getErr).To(MatchError(ContainSubstring("system:serviceaccount:")))
				})
			})
		})
	})

	Describe("Authorized In", func() {
		BeforeEach(func() {
			org1NS = createNamespace("org1", map[string]string{korifiv1alpha1.OrgNameKey: "org1"})
			org2NS = createNamespace("org2", map[string]string{korifiv1alpha1.OrgNameKey: "org2"})
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: org1NS}})).To(Succeed())
			Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: org2NS}})).To(Succeed())
		})

		When("a user is authenticated", func() {
			BeforeEach(func() {
				createRoleBindingForUser(userName, roleName1, org1NS)
				createRoleBindingForUser("some-other-user", roleName1, org2NS)
			})

			When("the user has a rolebinding in the namespace", func() {
				It("returns true", func() {
					authorized, err := nsPerms.AuthorizedIn(ctx, userIdentity, org1NS)
					Expect(err).NotTo(HaveOccurred())
					Expect(authorized).To(BeTrue())
				})
			})

			When("the user does not have a RoleBinding in the namespace", func() {
				It("returns false", func() {
					authorized, err := nsPerms.AuthorizedIn(ctx, userIdentity, org2NS)
					Expect(err).NotTo(HaveOccurred())
					Expect(authorized).To(BeFalse())
				})
			})
		})

		When("a service account is authenticated", func() {
			BeforeEach(func() {
				createRoleBindingForServiceAccount(serviceAccountName, serviceAccountNS, roleName1, org1NS)
				createRoleBindingForServiceAccount("some-other-service-account", serviceAccountNS, roleName1, org2NS)
				createRoleBindingForServiceAccount(serviceAccountName, "other-ns", roleName2, org2NS)
			})

			When("the service account has a rolebinding in the namespace", func() {
				It("returns true", func() {
					authorized, err := nsPerms.AuthorizedIn(ctx, serviceAccountIdentity, org1NS)
					Expect(err).NotTo(HaveOccurred())
					Expect(authorized).To(BeTrue())
				})
			})

			When("the service account does not have a RoleBinding in the namespace", func() {
				It("returns false", func() {
					authorized, err := nsPerms.AuthorizedIn(ctx, serviceAccountIdentity, org2NS)
					Expect(err).NotTo(HaveOccurred())
					Expect(authorized).To(BeFalse())
				})
			})

			When("the identity has an invalid service account name", func() {
				BeforeEach(func() {
					serviceAccountIdentity.Name = "name-without-prefix"
				})

				It("returns an error", func() {
					_, err := nsPerms.AuthorizedIn(ctx, serviceAccountIdentity, org2NS)
					Expect(err).To(MatchError(ContainSubstring("system:serviceaccount:")))
				})
			})
		})
	})
})

func generateGUID(prefix string) string {
	guid := uuid.NewString()
	return fmt.Sprintf("%s-%s", prefix, guid[:6])
}
