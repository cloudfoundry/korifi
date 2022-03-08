package e2e_test

import (
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("Service Instances", func() {
	var (
		orgGUID      string
		spaceGUID    string
		instanceGUID string
		instanceName string
		httpResp     *resty.Response
		httpError    error
	)

	BeforeEach(func() {
		orgGUID = createOrg(generateGUID("org"))
		spaceGUID = createSpace(generateGUID("space1"), orgGUID)
		createOrgRole("organization_user", rbacv1.UserKind, certUserName, orgGUID)
		instanceName = generateGUID("service-instance")
		instanceGUID = createServiceInstance(spaceGUID, instanceName)
	})

	AfterEach(func() {
		deleteOrg(orgGUID)
	})

	Describe("Create", func() {
		When("the service instance name is not unique", func() {
			JustBeforeEach(func() {
				httpResp, httpError = adminClient.R().
					SetBody(serviceInstanceResource{
						resource: resource{
							Name: instanceName,
							Relationships: relationships{
								"space": {
									Data: resource{
										GUID: spaceGUID,
									},
								},
							},
						},
						InstanceType: "user-provided",
					}).Post("/v3/service_instances")
			})
			It("fails", func() {
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
				Expect(httpResp).To(HaveRestyBody(ContainSubstring(fmt.Sprintf("The service instance name is taken: %s", instanceName))))
			})
		})
	})

	Describe("Delete", func() {
		JustBeforeEach(func() {
			httpResp, httpError = certClient.R().Delete("/v3/service_instances/" + instanceGUID)
		})

		It("fails with 404 Not Found", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusNotFound))
		})

		When("the user has permissions to delete service instances", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("succeeds", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusNoContent))
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

		When("the user has read only permissions over service instances", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", rbacv1.UserKind, certUserName, spaceGUID)
			})

			It("fails with 403 Forbidden", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusForbidden))
			})
		})
	})
})
