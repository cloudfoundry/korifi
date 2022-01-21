package e2e_test

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Roles", func() {
	var (
		ctx      context.Context
		userName string
		org      presenter.OrgResponse
	)

	BeforeEach(func() {
		ctx = context.Background()
		userName = generateGUID("user")

		org = createOrg(generateGUID("org"), adminAuthHeader)
	})

	AfterEach(func() {
		deleteSubnamespace(rootNamespace, org.GUID)
	})

	Describe("creating an org role", func() {
		var role presenter.RoleResponse

		BeforeEach(func() {
			resp, err := api.NewRequest().
				SetHeader(headers.Authorization, tokenAuthHeader).
				SetBody(userOrgRolePayload("organization_manager", userName, org.GUID)).
				SetResult(&role).
				Post("/v3/roles")

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(HaveHTTPStatus(http.StatusCreated))
		})

		It("creates a role binding", func() {
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
			space presenter.SpaceResponse
			role  presenter.RoleResponse
		)

		BeforeEach(func() {
			createOrgRole("organization_user", rbacv1.UserKind, userName, org.GUID, adminAuthHeader)
			space := createSpace(generateGUID("space"), org.GUID, adminAuthHeader)

			api.NewRequest().
				SetHeader(headers.Authorization, adminAuthHeader).
				SetBody(userSpaceRolePayload("space_developer", userName, space.GUID)).
				SetResult(&role).
				Post("/v3/roles")
		})

		AfterEach(func() {
			deleteSubnamespace(org.GUID, space.GUID)
		})

		It("creates a role binding", func() {
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
