package e2e_test

import (
	"context"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
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

		api.Request(http.MethodPost, "/v3/organizations").
			WithBody(orgPayload(generateGUID("org"))).
			DoWithAuth(adminAuthHeader).
			ValidateStatus(http.StatusCreated).
			DecodeResponseBody(&org)
	})

	AfterEach(func() {
		deleteSubnamespace(rootNamespace, org.GUID)
	})

	Describe("creating an org role", func() {
		var role presenter.RoleResponse

		BeforeEach(func() {
			api.Request(http.MethodPost, "/v3/roles").
				WithBody(userOrgRolePayload("organization_manager", userName, org.GUID)).
				DoWithAuth(tokenAuthHeader).
				ValidateStatus(http.StatusCreated).
				DecodeResponseBody(&role)
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
			api.Request(http.MethodPost, "/v3/roles").
				WithBody(userOrgRolePayload("organization_user", userName, org.GUID)).
				DoWithAuth(adminAuthHeader).
				ValidateStatus(http.StatusCreated)

			api.Request(http.MethodPost, "/v3/spaces").
				WithBody(spacePayload(generateGUID("space"), org.GUID)).
				DoWithAuth(adminAuthHeader).
				ValidateStatus(http.StatusCreated).
				DecodeResponseBody(&space)

			api.Request(http.MethodPost, "/v3/roles").
				WithBody(userSpaceRolePayload("space_developer", userName, space.GUID)).
				DoWithAuth(adminAuthHeader).
				ValidateStatus(http.StatusCreated).
				DecodeResponseBody(&role)
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
