package e2e_test

import (
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Service Instances", func() {
	var (
		spaceGUID            string
		existingInstanceGUID string
		existingInstanceName string
		httpResp             *resty.Response
		httpError            error
	)

	BeforeEach(func() {
		spaceGUID = createSpace(generateGUID("space1"), commonTestOrgGUID)
		existingInstanceName = generateGUID("service-instance")
		existingInstanceGUID = createServiceInstance(spaceGUID, existingInstanceName, nil)
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	Describe("Create", func() {
		When("the user has permissions to create service instances", func() {
			var instanceName string

			BeforeEach(func() {
				instanceName = generateGUID("service-instance")
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			JustBeforeEach(func() {
				httpResp, httpError = certClient.R().
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
						Credentials: map[string]string{
							"type":  "database",
							"hello": "creds",
						},
						InstanceType: "user-provided",
						Tags:         []string{"some", "tags"},
					}).Post("/v3/service_instances")
			})

			It("succeeds", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusCreated))

				serviceInstances := listServiceInstances(instanceName)
				Expect(serviceInstances.Resources).To(HaveLen(1))

				serviceInstance := serviceInstances.Resources[0]
				Expect(serviceInstance.Name).To(Equal(instanceName))
				Expect(serviceInstance.Relationships["space"].Data.GUID).To(Equal(spaceGUID))
				Expect(serviceInstance.Tags).To(ConsistOf("some", "tags"))
				Expect(serviceInstance.InstanceType).To(Equal("user-provided"))
			})
		})

		When("the service instance name is not unique", func() {
			JustBeforeEach(func() {
				httpResp, httpError = adminClient.R().
					SetBody(serviceInstanceResource{
						resource: resource{
							Name: existingInstanceName,
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
				Expect(httpResp).To(HaveRestyBody(ContainSubstring(fmt.Sprintf("The service instance name is taken: %s", existingInstanceName))))
			})
		})
	})

	Describe("Create managed", func() {
		var instanceName string

		BeforeEach(func() {
			instanceName = generateGUID("service-instance")
			createSpaceRole("space_developer", certUserName, spaceGUID)
		})

		JustBeforeEach(func() {
			httpResp, httpError = certClient.R().
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
					Credentials: map[string]string{
						"type":  "database",
						"hello": "creds",
					},
					InstanceType: "managed",
				}).Post("/v3/service_instances")
		})

		It("succeeds", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusCreated))

			serviceInstances := listServiceInstances(instanceName)
			Expect(serviceInstances.Resources).To(HaveLen(1))

			serviceInstance := serviceInstances.Resources[0]
			Expect(serviceInstance.Name).To(Equal(instanceName))
			Expect(serviceInstance.InstanceType).To(Equal("managed"))
		})
	})

	Describe("Update", func() {
		When("the user has permissions to update service instances", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			JustBeforeEach(func() {
				httpResp, httpError = certClient.R().
					SetBody(serviceInstanceResource{
						resource: resource{
							Name: "new-instance-name",
							Metadata: &metadata{
								Labels:      map[string]string{"a-label": "a-label-value"},
								Annotations: map[string]string{"an-annotation": "an-annotation-value"},
							},
						},
						Credentials: map[string]string{
							"bye": "new-creds",
						},
						Tags: []string{"some", "tags"},
					}).Patch("/v3/service_instances/" + existingInstanceGUID)
			})

			It("succeeds", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))

				serviceInstances := listServiceInstances("new-instance-name")
				Expect(serviceInstances.Resources).To(HaveLen(1))

				serviceInstance := serviceInstances.Resources[0]
				Expect(serviceInstance.Name).To(Equal("new-instance-name"))
				Expect(serviceInstance.Metadata.Labels).To(Equal(map[string]string{"a-label": "a-label-value"}))
				Expect(serviceInstance.Metadata.Annotations).To(Equal(map[string]string{"an-annotation": "an-annotation-value"}))
				Expect(serviceInstance.Tags).To(ConsistOf("some", "tags"))
			})
		})
	})

	Describe("Delete", func() {
		JustBeforeEach(func() {
			httpResp, httpError = certClient.R().Delete("/v3/service_instances/" + existingInstanceGUID)
		})

		It("fails with 404 Not Found", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusNotFound))
		})

		When("the user has permissions to delete service instances", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			It("succeeds", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusNoContent))
			})

			It("deletes the service instance", func() {
				Expect(listServiceInstances().Resources).NotTo(ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal(existingInstanceName),
						"GUID": Equal(existingInstanceGUID),
					}),
				))
			})
		})

		When("the user has read only permissions over service instances", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", certUserName, spaceGUID)
			})

			It("fails with 403 Forbidden", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusForbidden))
			})
		})
	})
})
