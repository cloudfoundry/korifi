package e2e_test

import (
	"net/http"
	"strings"

	"code.cloudfoundry.org/korifi/tests/helpers/broker"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Service Instances", func() {
	var (
		spaceGUID         string
		upsiGUID          string
		upsiWithCredsGUID string
		upsiName          string
		// managedName       string
		// managedGUID       string
		httpResp  *resty.Response
		httpError error
	)

	BeforeEach(func() {
		spaceGUID = createSpace(generateGUID("space1"), commonTestOrgGUID)
		upsiName = generateGUID("upsi-service-instance")
		upsiWithCredsGUID = generateGUID("upsi-service-instance-creds")
		upsiGUID = createUPServiceInstance(spaceGUID, upsiName, nil)
		// managedName = generateGUID("managed-service-instance")
		// managedGUID = createManagedServiceInstance(spaceGUID, managedName)
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	Describe("Get", func() {
		var result serviceInstanceResource

		BeforeEach(func() {
			httpResp, httpError = adminClient.R().SetResult(&result).Get("/v3/service_instances/" + upsiGUID)
		})

		It("gets the service instance", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))

			Expect(result.GUID).To(Equal(upsiGUID))
			Expect(result.Name).To(Equal(upsiName))
		})
	})

	Describe("GetCredentials", func() {
		var result map[string]any

		BeforeEach(func() {
			upsiWithCredsGUID = createUPServiceInstance(spaceGUID, generateGUID("service-instance2"), map[string]string{"a": "b"})
		})

		JustBeforeEach(func() {
			httpResp, httpError = adminClient.R().SetResult(&result).Get("/v3/service_instances/" + upsiWithCredsGUID + "/credentials")
		})

		It("returns the service instance credentials", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))

			Expect(result).To(Equal(map[string]any{"a": "b"}))
		})
	})

	Describe("Create", func() {
		var (
			instanceName  string
			createPayload serviceInstanceResource
			result        serviceInstanceResource
		)

		BeforeEach(func() {
			instanceName = generateGUID("service-instance")
			createPayload = serviceInstanceResource{}
		})

		JustBeforeEach(func() {
			httpResp, httpError = adminClient.R().
				SetBody(createPayload).
				SetResult(&result).
				Post("/v3/service_instances")
		})

		When("creating a user-provided service instance", func() {
			BeforeEach(func() {
				createPayload = serviceInstanceResource{
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
					Credentials: map[string]any{
						"object": map[string]any{"a": "b"},
					},
					InstanceType: "user-provided",
				}
			})

			It("succeeds", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusCreated))
				Expect(result.Name).To(Equal(instanceName))
				Expect(result.InstanceType).To(Equal("user-provided"))
			})
		})

		When("creating a managed service instance", func() {
			var brokerGUID string

			BeforeEach(func() {
				brokerGUID = createBroker(serviceBrokerURL)

				var plansResp resourceList[resource]
				catalogResp, err := adminClient.R().SetResult(&plansResp).Get("/v3/service_plans?service_broker_guids=" + brokerGUID)
				Expect(err).NotTo(HaveOccurred())
				Expect(catalogResp).To(HaveRestyStatusCode(http.StatusOK))
				Expect(plansResp.Resources).NotTo(BeEmpty())

				createPayload = serviceInstanceResource{
					resource: resource{
						Name: instanceName,
						Relationships: relationships{
							"space": {
								Data: resource{
									GUID: spaceGUID,
								},
							},
							"service_plan": {
								Data: resource{
									GUID: plansResp.Resources[0].GUID,
								},
							},
						},
					},
					InstanceType: "managed",
				}
			})

			AfterEach(func() {
				broker.NewDeleter(rootNamespace).ForBrokerGUID(brokerGUID).Delete()
			})

			It("succeeds with a job redirect", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusAccepted))

				Expect(httpResp).To(SatisfyAll(
					HaveRestyStatusCode(http.StatusAccepted),
					HaveRestyHeaderWithValue("Location", ContainSubstring("/v3/jobs/managed_service_instance.create~")),
				))
				expectJobCompletes(httpResp)
			})
		})
	})

	Describe("Update", func() {
		JustBeforeEach(func() {
			httpResp, httpError = adminClient.R().
				SetBody(serviceInstanceResource{
					resource: resource{
						Name: "new-instance-name",
						Metadata: &metadata{
							Labels:      map[string]string{"a-label": "a-label-value"},
							Annotations: map[string]string{"an-annotation": "an-annotation-value"},
						},
					},
					Credentials: map[string]any{
						"object-new": map[string]any{"new-a": "new-b"},
					},
					Tags: []string{"some", "tags"},
				}).Patch("/v3/service_instances/" + upsiGUID)
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
		var serviceInstanceGUID string

		JustBeforeEach(func() {
			httpResp, httpError = adminClient.R().Delete("/v3/service_instances/" + serviceInstanceGUID)
		})

		When("deleting a user-provided service instance", func() {
			BeforeEach(func() {
				serviceInstanceGUID = upsiGUID
			})

			It("responds with deletion job location", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusNoContent))
				Expect(listServiceInstances().Resources).NotTo(ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"Name": Equal(upsiName),
						"GUID": Equal(upsiGUID),
					}),
				))
			})
		})

		When("deleting a managed service instance", func() {
			var brokerGUID string

			BeforeEach(func() {
				brokerGUID = createBroker(serviceBrokerURL)

				serviceInstanceGUID = createManagedServiceInstance(brokerGUID, spaceGUID)
			})

			AfterEach(func() {
				broker.NewDeleter(rootNamespace).ForBrokerGUID(brokerGUID).Delete()
			})

			It("succeeds with a job redirect", func() {
				Expect(httpError).NotTo(HaveOccurred())
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusAccepted))

				Expect(httpResp).To(SatisfyAll(
					HaveRestyStatusCode(http.StatusAccepted),
					HaveRestyHeaderWithValue("Location", ContainSubstring("/v3/jobs/managed_service_instance.delete~")),
				))
				expectJobCompletes(httpResp)
			})
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
			anotherInstanceGUID = createUPServiceInstance(anotherSpaceGUID, generateGUID("service-instance"), nil)
		})

		JustBeforeEach(func() {
			serviceInstancesList = resourceList[resource]{}
			httpResp, httpError = adminClient.R().SetResult(&serviceInstancesList).Get("/v3/service_instances")
		})

		It("lists the service instances", func() {
			Expect(httpError).NotTo(HaveOccurred())
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(serviceInstancesList.Resources).To(ContainElements(
				MatchFields(IgnoreExtras, Fields{
					"GUID": Equal(upsiGUID),
				}),
				MatchFields(IgnoreExtras, Fields{
					"GUID": Equal(anotherInstanceGUID),
				}),
			))
		})
	})
})

func createManagedServiceInstance(brokerGUID, spaceGUID string) string {
	GinkgoHelper()

	var plansResp resourceList[resource]
	catalogResp, err := adminClient.R().SetResult(&plansResp).Get("/v3/service_plans?service_broker_guids=" + brokerGUID)
	Expect(err).NotTo(HaveOccurred())
	Expect(catalogResp).To(HaveRestyStatusCode(http.StatusOK))
	Expect(plansResp.Resources).NotTo(BeEmpty())

	createPayload := serviceInstanceResource{
		resource: resource{
			Name: uuid.NewString(),
			Relationships: relationships{
				"space": {
					Data: resource{
						GUID: spaceGUID,
					},
				},
				"service_plan": {
					Data: resource{
						GUID: plansResp.Resources[0].GUID,
					},
				},
			},
		},
		InstanceType: "managed",
	}

	var result serviceInstanceResource
	httpResp, httpError := adminClient.R().
		SetBody(createPayload).
		SetResult(&result).
		Post("/v3/service_instances")
	Expect(httpError).NotTo(HaveOccurred())
	Expect(httpResp).To(SatisfyAll(
		HaveRestyStatusCode(http.StatusAccepted),
		HaveRestyHeaderWithValue("Location", ContainSubstring("/v3/jobs/managed_service_instance.create~")),
	))
	jobURL := httpResp.Header().Get("Location")
	expectJobCompletes(httpResp)

	return strings.Split(jobURL, "~")[1]
}
