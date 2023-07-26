package e2e_test

import (
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
	})

	Describe("Update", func() {
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
			Expect(serviceInstance.Metadata.Labels).To(HaveKeyWithValue("a-label", "a-label-value"))
			Expect(serviceInstance.Metadata.Annotations).To(HaveKeyWithValue("an-annotation", "an-annotation-value"))
			Expect(serviceInstance.Tags).To(ConsistOf("some", "tags"))
		})
	})

	Describe("Delete", func() {
		BeforeEach(func() {
			createSpaceRole("space_developer", certUserName, spaceGUID)
		})

		JustBeforeEach(func() {
			httpResp, httpError = certClient.R().Delete("/v3/service_instances/" + existingInstanceGUID)
		})

		It("deletes the service instance", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusNoContent))
			Expect(listServiceInstances().Resources).NotTo(ContainElement(
				MatchFields(IgnoreExtras, Fields{
					"Name": Equal(existingInstanceName),
					"GUID": Equal(existingInstanceGUID),
				}),
			))
		})
	})

	Describe("List", func() {
		var (
			anotherSpaceGUID     string
			anotherInstanceGUID  string
			serviceInstancesList resourceList[resource]
		)

		BeforeEach(func() {
			anotherSpaceGUID = createSpace(generateGUID("space1"), commonTestOrgGUID)
			anotherInstanceGUID = createServiceInstance(anotherSpaceGUID, generateGUID("service-instance"), nil)

			createSpaceRole("space_developer", certUserName, spaceGUID)
			createSpaceRole("space_developer", certUserName, anotherSpaceGUID)
		})

		JustBeforeEach(func() {
			serviceInstancesList = resourceList[resource]{}
			httpResp, httpError = certClient.R().SetResult(&serviceInstancesList).Get("/v3/service_instances")
		})

		It("lists the service instances", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(serviceInstancesList.Resources).To(ContainElements(
				MatchFields(IgnoreExtras, Fields{
					"GUID": Equal(existingInstanceGUID),
				}),
				MatchFields(IgnoreExtras, Fields{
					"GUID": Equal(anotherInstanceGUID),
				}),
			))
		})
	})
})
