package authorization_test

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/cf-k8s-api/repositories/authorization"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Org", func() {
	var (
		ctx        context.Context
		org        *authorization.Org
		namespaces []string
		getErr     error
		identity   authorization.Identity

		org1Ns, org2Ns string
		roleName       string
	)

	createNamespace := func() string {
		guid := uuid.NewString()
		Expect(k8sClient.Create(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: guid}})).To(Succeed())

		return guid
	}

	createClusterRole := func(name string) *rbacv1.ClusterRole {
		role := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}

		Expect(k8sClient.Create(context.Background(), role)).To(Succeed())

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

		Expect(k8sClient.Create(context.Background(), role)).To(Succeed())

		return role
	}

	BeforeEach(func() {
		userName := generateGUID("alice")
		ctx = context.Background()
		org = authorization.NewOrg(k8sClient)
		identity = authorization.Identity{
			Kind: rbacv1.UserKind,
			Name: userName,
		}

		org1Ns = createNamespace()
		org2Ns = createNamespace()

		roleName = generateGUID("org-user")
		createClusterRole(roleName)
		createRoleBindingForUser(userName, roleName, org1Ns)
		createRoleBindingForUser("some-other-user", roleName, org1Ns)
	})

	JustBeforeEach(func() {
		namespaces, getErr = org.GetAuthorizedNamespaces(ctx, identity)
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: org1Ns}})).To(Succeed())
		Expect(k8sClient.Delete(context.Background(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: org2Ns}})).To(Succeed())
		Expect(k8sClient.Delete(context.Background(), &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: roleName}})).To(Succeed())
	})

	It("lists the namespaces with bindings for current user", func() {
		Expect(getErr).NotTo(HaveOccurred())
		Expect(namespaces).To(ConsistOf(org1Ns))
	})

	When("the user does not have a rolebinding associated with it", func() {
		BeforeEach(func() {
			identity = authorization.Identity{
				Name: generateGUID("bob"),
				Kind: "User",
			}
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

func generateGUID(prefix string) string {
	guid := uuid.NewString()
	return fmt.Sprintf("%s-%s", prefix, guid[:6])
}
