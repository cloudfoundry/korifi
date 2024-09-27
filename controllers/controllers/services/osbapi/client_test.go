package osbapi_test

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tests/helpers/broker"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("OSBAPI Client", func() {
	var (
		brokerClient *osbapi.Client
		brokerServer *broker.BrokerServer
	)

	BeforeEach(func() {
		brokerServer = broker.NewServer()
	})

	JustBeforeEach(func() {
		brokerServer.Start()
		DeferCleanup(func() {
			brokerServer.Stop()
		})

		brokerClient = osbapi.NewClient(osbapi.Broker{
			URL:      brokerServer.URL(),
			Username: "broker-user",
			Password: "broker-password",
		}, &http.Client{Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //#nosec G402
		}})
	})

	Describe("GetCatalog", func() {
		var (
			catalog       osbapi.Catalog
			getCatalogErr error
		)

		BeforeEach(func() {
			brokerServer.WithResponse(
				"/v2/catalog",
				map[string]any{
					"services": []map[string]any{
						{
							"id":          "123456",
							"name":        "test-service",
							"description": "test service description",
							"bindable":    true,
						},
					},
				},
				http.StatusOK,
			)
		})

		JustBeforeEach(func() {
			catalog, getCatalogErr = brokerClient.GetCatalog(ctx)
		})

		It("gets the catalog", func() {
			Expect(getCatalogErr).NotTo(HaveOccurred())
			Expect(catalog).To(Equal(osbapi.Catalog{
				Services: []osbapi.Service{{
					ID:          "123456",
					Name:        "test-service",
					Description: "test service description",
					BrokerCatalogFeatures: services.BrokerCatalogFeatures{
						Bindable: true,
					},
				}},
			}))
		})

		It("sends a sync request", func() {
			servedRequests := brokerServer.ServedRequests()
			Expect(servedRequests).To(HaveLen(1))
			Expect(servedRequests[0].Method).To(Equal(http.MethodGet))
			Expect(servedRequests[0].URL.Query().Get("accepts_incomplete")).To(BeEmpty())
		})

		It("sends broker credentials in the Authorization request header", func() {
			Expect(brokerServer.ServedRequests()).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Header": HaveKeyWithValue(
					"Authorization", ConsistOf("Basic "+base64.StdEncoding.EncodeToString([]byte("broker-user:broker-password"))),
				),
			}))))
		})

		It("sends OSBAPI version request header", func() {
			Expect(brokerServer.ServedRequests()).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Header": HaveKeyWithValue(
					"X-Broker-Api-Version", ConsistOf("2.17"),
				),
			}))))
		})

		When("getting the catalog fails", func() {
			BeforeEach(func() {
				brokerServer = broker.NewServer().WithHandler("/v2/catalog", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusTeapot)
				}))
			})

			It("returns an error", func() {
				Expect(getCatalogErr).To(MatchError(ContainSubstring(strconv.Itoa(http.StatusTeapot))))
			})
		})

		When("the catalog response cannot be unmarshalled", func() {
			BeforeEach(func() {
				brokerServer = broker.NewServer().WithHandler("/v2/catalog", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					_, _ = w.Write([]byte("hello"))
				}))
			})

			It("returns an error", func() {
				Expect(getCatalogErr).To(MatchError(ContainSubstring("failed to unmarshal catalog")))
			})
		})
	})

	Describe("Instances", func() {
		Describe("Provision", func() {
			var (
				provisionResp osbapi.ServiceInstanceOperationResponse
				provisionErr  error
			)

			BeforeEach(func() {
				brokerServer = broker.NewServer().WithResponse(
					"/v2/service_instances/{id}",
					map[string]any{
						"operation": "provision_op1",
					},
					http.StatusCreated,
				)
			})

			JustBeforeEach(func() {
				provisionResp, provisionErr = brokerClient.Provision(ctx, osbapi.InstanceProvisionPayload{
					InstanceID: "my-service-instance",
					InstanceProvisionRequest: osbapi.InstanceProvisionRequest{
						ServiceId: "service-guid",
						PlanID:    "plan-guid",
						SpaceGUID: "space-guid",
						OrgGUID:   "org-guid",
						Parameters: map[string]any{
							"foo": "bar",
						},
					},
				})
			})

			It("sends async provision request to broker", func() {
				Expect(provisionErr).NotTo(HaveOccurred())
				requests := brokerServer.ServedRequests()

				Expect(requests).To(HaveLen(1))

				Expect(requests[0].Method).To(Equal(http.MethodPut))
				Expect(requests[0].URL.Path).To(Equal("/v2/service_instances/my-service-instance"))

				Expect(requests[0].URL.Query().Get("accepts_incomplete")).To(Equal("true"))
			})

			It("sends correct request body", func() {
				Expect(provisionErr).NotTo(HaveOccurred())
				requests := brokerServer.ServedRequests()

				Expect(requests).To(HaveLen(1))

				requestBytes, err := io.ReadAll(requests[0].Body)
				Expect(err).NotTo(HaveOccurred())
				requestBody := map[string]any{}
				Expect(json.Unmarshal(requestBytes, &requestBody)).To(Succeed())

				Expect(requestBody).To(MatchAllKeys(Keys{
					"service_id":        Equal("service-guid"),
					"plan_id":           Equal("plan-guid"),
					"space_guid":        Equal("space-guid"),
					"organization_guid": Equal("org-guid"),
					"parameters": MatchAllKeys(Keys{
						"foo": Equal("bar"),
					}),
				}))
			})

			It("provisions the service synchronously", func() {
				Expect(provisionErr).NotTo(HaveOccurred())
				Expect(provisionResp).To(Equal(osbapi.ServiceInstanceOperationResponse{
					Operation: "provision_op1",
					Complete:  true,
				}))
			})

			When("the broker accepts the provision request", func() {
				BeforeEach(func() {
					brokerServer = broker.NewServer().WithResponse(
						"/v2/service_instances/{id}",
						map[string]any{
							"operation": "provision_op1",
						},
						http.StatusAccepted,
					)
				})

				It("provisions the service asynchronously", func() {
					Expect(provisionErr).NotTo(HaveOccurred())
					Expect(provisionResp).To(Equal(osbapi.ServiceInstanceOperationResponse{
						Operation: "provision_op1",
						Complete:  false,
					}))
				})
			})

			When("the provision request fails", func() {
				BeforeEach(func() {
					brokerServer = broker.NewServer().WithHandler("/v2/service_instances/{id}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusTeapot)
					}))
				})

				It("returns an error", func() {
					Expect(provisionErr).To(MatchError(ContainSubstring("provision request failed")))
				})
			})
		})

		Describe("Deprovision", func() {
			var (
				deprovisionResp osbapi.ServiceInstanceOperationResponse
				deprovisionErr  error
			)

			BeforeEach(func() {
				brokerServer.WithResponse(
					"/v2/service_instances/{id}",
					map[string]any{
						"operation": "provision_op1",
					},
					http.StatusOK,
				)
			})

			JustBeforeEach(func() {
				deprovisionResp, deprovisionErr = brokerClient.Deprovision(ctx, osbapi.InstanceDeprovisionPayload{
					ID: "my-service-instance",
					InstanceDeprovisionRequest: osbapi.InstanceDeprovisionRequest{
						ServiceId: "service-guid",
						PlanID:    "plan-guid",
					},
				})
			})

			It("deprovisions the service", func() {
				Expect(deprovisionErr).NotTo(HaveOccurred())
				Expect(deprovisionResp).To(Equal(osbapi.ServiceInstanceOperationResponse{
					Operation: "provision_op1",
				}))
			})

			It("sends async deprovision request to broker", func() {
				Expect(deprovisionErr).NotTo(HaveOccurred())
				requests := brokerServer.ServedRequests()

				Expect(requests).To(HaveLen(1))

				Expect(requests[0].Method).To(Equal(http.MethodDelete))
				Expect(requests[0].URL.Path).To(Equal("/v2/service_instances/my-service-instance"))

				Expect(requests[0].URL.Query().Get("accepts_incomplete")).To(Equal("true"))
			})

			It("sends correct request body", func() {
				Expect(deprovisionErr).NotTo(HaveOccurred())
				requests := brokerServer.ServedRequests()

				Expect(requests).To(HaveLen(1))

				requestBytes, err := io.ReadAll(requests[0].Body)
				Expect(err).NotTo(HaveOccurred())
				requestBody := map[string]any{}
				Expect(json.Unmarshal(requestBytes, &requestBody)).To(Succeed())

				Expect(requestBody).To(MatchAllKeys(Keys{
					"service_id": Equal("service-guid"),
					"plan_id":    Equal("plan-guid"),
				}))
			})

			When("the deprovision request fails", func() {
				BeforeEach(func() {
					brokerServer = broker.NewServer().WithHandler("/v2/service_instances/{id}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusTeapot)
					}))
				})

				It("returns an error", func() {
					Expect(deprovisionErr).To(MatchError(ContainSubstring("deprovision request failed")))
				})
			})
		})

		Describe("GetLastOperation", func() {
			var (
				lastOpResp osbapi.LastOperationResponse
				lastOpErr  error
			)

			BeforeEach(func() {
				brokerServer.WithResponse(
					"/v2/service_instances/{id}/last_operation",
					map[string]any{
						"state":       "in-progress",
						"description": "provisioning",
					},
					http.StatusOK,
				)
			})

			JustBeforeEach(func() {
				lastOpResp, lastOpErr = brokerClient.GetServiceInstanceLastOperation(ctx, osbapi.GetLastOperationPayload{
					ID: "my-service-instance",
					GetLastOperationRequest: osbapi.GetLastOperationRequest{
						ServiceId: "service-guid",
						PlanID:    "plan-guid",
						Operation: "op-guid",
					},
				})
			})

			It("gets the last operation", func() {
				Expect(lastOpErr).NotTo(HaveOccurred())
				Expect(lastOpResp).To(Equal(osbapi.LastOperationResponse{
					State:       "in-progress",
					Description: "provisioning",
				}))
			})

			It("sends correct request to broker", func() {
				Expect(lastOpErr).NotTo(HaveOccurred())
				requests := brokerServer.ServedRequests()

				Expect(requests).To(HaveLen(1))

				Expect(requests[0].Method).To(Equal(http.MethodGet))
				Expect(requests[0].URL.Path).To(Equal("/v2/service_instances/my-service-instance/last_operation"))

				requestBytes, err := io.ReadAll(requests[0].Body)
				Expect(err).NotTo(HaveOccurred())
				requestBody := map[string]any{}
				Expect(json.Unmarshal(requestBytes, &requestBody)).To(Succeed())

				Expect(requestBody).To(MatchAllKeys(Keys{
					"service_id": Equal("service-guid"),
					"plan_id":    Equal("plan-guid"),
					"operation":  Equal("op-guid"),
				}))
			})

			When("getting the last operation request fails", func() {
				BeforeEach(func() {
					brokerServer = broker.NewServer().WithHandler("/v2/service_instances/{id}/last_operation", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusTeapot)
					}))
				})

				It("returns an error", func() {
					Expect(lastOpErr).To(MatchError(ContainSubstring("getting last operation request failed")))
				})
			})

			When("getting the last operation request fails with 410 Gone", func() {
				BeforeEach(func() {
					brokerServer = broker.NewServer().WithHandler("/v2/service_instances/{id}/last_operation", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusGone)
					}))
				})

				It("returns a gone error", func() {
					Expect(lastOpErr).To(BeAssignableToTypeOf(osbapi.GoneError{}))
				})
			})
		})
	})
})
