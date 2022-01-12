package e2e_test

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Roles", func() {
	var (
		ctx      context.Context
		userName string
	)

	BeforeEach(func() {
		ctx = context.Background()
		userName = uuid.NewString()
	})

	Describe("creating an org role", func() {
		var org presenter.OrgResponse

		BeforeEach(func() {
			org = createOrg(uuid.NewString(), adminAuthHeader)
		})

		AfterEach(func() {
			deleteSubnamespace(rootNamespace, org.GUID)
		})

		It("creates a role binding", func() {
			role := createOrgRole("organization_manager", rbacv1.UserKind, userName, org.GUID, tokenAuthHeader)

			binding := getOrgRoleBinding(ctx, org.GUID, role.GUID)
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
			org = createOrg(uuid.NewString(), adminAuthHeader)
			createOrgRole("organization_user", rbacv1.UserKind, userName, org.GUID, adminAuthHeader)
			space = createSpace(uuid.NewString(), org.GUID, adminAuthHeader)
		})

		AfterEach(func() {
			deleteSubnamespace(org.GUID, space.GUID)
			deleteSubnamespace(rootNamespace, org.GUID)
		})

		It("creates a role binding", func() {
			role := createSpaceRole("space_developer", rbacv1.UserKind, userName, space.GUID, tokenAuthHeader)

			binding := getOrgRoleBinding(ctx, space.GUID, role.GUID)
			Expect(binding.RoleRef.Name).To(Equal("cf-k8s-controllers-space-developer"))
			Expect(binding.RoleRef.Kind).To(Equal("ClusterRole"))
			Expect(binding.Subjects).To(HaveLen(1))

			subject := binding.Subjects[0]
			Expect(subject.Name).To(Equal(userName))
			Expect(subject.Kind).To(Equal(rbacv1.UserKind))
		})
	})
})

func getOrgRoleBinding(ctx context.Context, orgGuid, roleGuid string) rbacv1.RoleBinding {
	roleBindingList := &rbacv1.RoleBindingList{}
	Expect(k8sClient.List(ctx, roleBindingList,
		client.InNamespace(orgGuid),
		client.MatchingLabels{repositories.RoleGuidLabel: roleGuid},
	)).To(Succeed())
	Expect(roleBindingList.Items).To(HaveLen(1))

	return roleBindingList.Items[0]
}
