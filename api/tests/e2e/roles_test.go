package e2e_test

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Roles", func() {
	var (
		ctx      context.Context
		userName string
	)

	createBinding := func(namespace, userName, roleName string) *rbacv1.RoleBinding {
		binding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: namespace,
			},
			Subjects: []rbacv1.Subject{
				{
					Kind: rbacv1.UserKind,
					Name: userName,
				},
			},
			RoleRef: rbacv1.RoleRef{
				Kind: "ClusterRole",
				Name: roleName,
			},
		}

		Expect(k8sClient.Create(ctx, binding)).To(Succeed())

		return binding
	}

	BeforeEach(func() {
		ctx = context.Background()
		userName = uuid.NewString()
	})

	Describe("creating an org role", func() {
		var org presenter.OrgResponse

		BeforeEach(func() {
			org = createOrg(uuid.NewString(), tokenAuthHeader)
		})

		AfterEach(func() {
			deleteSubnamespace(rootNamespace, org.GUID)
		})

		It("creates a role binding", func() {
			role := createOrgRole("organization_manager", rbacv1.UserKind, userName, org.GUID)
			Expect(role.Type).To(Equal("organization_manager"))

			roleBindingList := &rbacv1.RoleBindingList{}
			Eventually(func() ([]rbacv1.RoleBinding, error) {
				err := k8sClient.List(ctx, roleBindingList,
					client.InNamespace(org.GUID),
					client.MatchingLabels{
						repositories.RoleTypeLabel: "organization_manager",
					},
				)
				if err != nil {
					return nil, err
				}
				return roleBindingList.Items, nil
			}).Should(HaveLen(1))

			binding := roleBindingList.Items[0]
			Expect(role.GUID).To(Equal(binding.Labels[repositories.RoleGuidLabel]))
			Expect(binding.RoleRef.Name).To(Equal("cf-k8s-controllers-organization-manager"))
			Expect(binding.RoleRef.Kind).To(Equal("ClusterRole"))
			Expect(binding.Subjects).To(HaveLen(1))
			subject := binding.Subjects[0]
			Expect(subject.Name).To(Equal(userName))
			Expect(subject.Kind).To(Equal(rbacv1.UserKind))
		})
	})

	Describe("creating a space role", func() {
		var (
			org   presenter.OrgResponse
			space presenter.SpaceResponse
		)

		BeforeEach(func() {
			org = createOrg(uuid.NewString(), tokenAuthHeader)
			space = createSpace(uuid.NewString(), org.GUID, tokenAuthHeader)
			createBinding(org.GUID, userName, "basic-user")
		})

		AfterEach(func() {
			deleteSubnamespace(org.GUID, space.GUID)
			deleteSubnamespace(rootNamespace, org.GUID)
		})

		It("creates a role binding", func() {
			role := createSpaceRole("space_developer", rbacv1.UserKind, userName, space.GUID)

			Expect(role.Type).To(Equal("space_developer"))

			roleBindingList := &rbacv1.RoleBindingList{}
			Eventually(func() ([]rbacv1.RoleBinding, error) {
				err := k8sClient.List(ctx, roleBindingList,
					client.InNamespace(space.GUID),
					client.MatchingLabels{
						repositories.RoleTypeLabel: "space_developer",
					},
				)
				if err != nil {
					return nil, err
				}
				return roleBindingList.Items, nil
			}).Should(HaveLen(1))

			binding := roleBindingList.Items[0]
			Expect(role.GUID).To(Equal(binding.Labels[repositories.RoleGuidLabel]))
			Expect(binding.RoleRef.Name).To(Equal("cf-k8s-controllers-space-developer"))
			Expect(binding.RoleRef.Kind).To(Equal("ClusterRole"))
			Expect(binding.Subjects).To(HaveLen(1))
			subject := binding.Subjects[0]
			Expect(subject.Name).To(Equal(userName))
			Expect(subject.Kind).To(Equal(rbacv1.UserKind))
		})
	})
})
