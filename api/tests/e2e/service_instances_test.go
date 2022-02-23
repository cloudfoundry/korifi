package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Service Instances", func() {
	Describe("Delete", func() {
		var (
			orgGUID      string
			spaceGUID    string
			instanceGUID string
			httpResp     *resty.Response
			httpError    error
		)

		BeforeEach(func() {
			orgGUID = createOrg(generateGUID("org"))
			spaceGUID = createSpace(generateGUID("space1"), orgGUID)
			createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)
			instanceGUID = createServiceInstance(spaceGUID, generateGUID("service-instance"))
		})

		JustBeforeEach(func() {
			httpResp, httpError = certClient.R().Delete("/v3/service_instances/" + instanceGUID)
		})

		AfterEach(func() {
			deleteOrg(orgGUID)
		})

		It("fails with 404 Not Found", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp.StatusCode()).To(Equal(http.StatusNotFound))
		})

		When("the user has permissions to delete service instances", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("succeeds", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp.StatusCode()).To(Equal(http.StatusNoContent))
			})

			It("deletes the service instance", func() {
				serviceInstances := listServiceInstances()
				Expect(serviceInstances.Resources).NotTo(ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal(instanceGUID),
					})),
				)
			})
		})

		When("the user does not have permission to delete service instances", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("fails with 403 Forbidden", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp.StatusCode()).To(Equal(http.StatusForbidden))
			})
		})
	})
})
