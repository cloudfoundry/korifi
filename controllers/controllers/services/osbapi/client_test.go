package osbapi_test

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
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
					Bindable:    true,
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

		When("getting the catalog fails", func() {
			BeforeEach(func() {
				brokerServer = brokerServer.WithResponse("/v2/catalog", nil, http.StatusTeapot)
			})

			It("returns an error", func() {
				Expect(getCatalogErr).To(MatchError(ContainSubstring(strconv.Itoa(http.StatusTeapot))))
			})
		})
	})

	Describe("Instances", func() {
		Describe("Provision", func() {
			var (
				provisionResp osbapi.ProvisionResponse
				provisionErr  error
			)

			BeforeEach(func() {
				brokerServer = brokerServer.WithResponse(
					"/v2/service_instances/{id}",
					nil,
					http.StatusCreated,
				)
			})

			JustBeforeEach(func() {
				provisionResp, provisionErr = brokerClient.Provision(ctx, osbapi.ProvisionPayload{
					InstanceID: "my-service-instance",
					ProvisionRequest: osbapi.ProvisionRequest{
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
				Expect(provisionResp).To(Equal(osbapi.ProvisionResponse{}))
			})

			When("the broker accepts the provision request", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse(
						"/v2/service_instances/{id}",
						map[string]any{
							"operation": "provision_op1",
						},
						http.StatusAccepted,
					)
				})

				It("provisions the service asynchronously", func() {
					Expect(provisionErr).NotTo(HaveOccurred())
					Expect(provisionResp).To(Equal(osbapi.ProvisionResponse{
						IsAsync:   true,
						Operation: "provision_op1",
					}))
				})
			})

			When("the provision request fails with 400 BadRequest error", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse("/v2/service_instances/{id}", nil, http.StatusBadRequest)
				})

				It("returns an unrecoverable error", func() {
					Expect(provisionErr).To(Equal(osbapi.UnrecoverableError{Status: http.StatusBadRequest}))
				})
			})

			When("the provision request fails with 409 Conflict error", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse("/v2/service_instances/{id}", nil, http.StatusConflict)
				})

				It("returns an unrecoverable error", func() {
					Expect(provisionErr).To(Equal(osbapi.UnrecoverableError{Status: http.StatusConflict}))
				})
			})

			When("the provision request fails with 422 Unprocessable entity error", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse("/v2/service_instances/{id}", nil, http.StatusUnprocessableEntity)
				})

				It("returns an unrecoverable error", func() {
					Expect(provisionErr).To(Equal(osbapi.UnrecoverableError{Status: http.StatusUnprocessableEntity}))
				})
			})

			When("the provision request fails", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse("/v2/service_instances/{id}", nil, http.StatusInternalServerError)
				})

				It("returns an error", func() {
					Expect(provisionErr).To(MatchError(ContainSubstring("provision request failed")))
				})
			})
		})

		Describe("Deprovision", func() {
			var (
				deprovisionResp osbapi.ProvisionResponse
				deprovisionErr  error
			)

			BeforeEach(func() {
				brokerServer.WithResponse(
					"/v2/service_instances/{id}",
					map[string]any{
						"operation": "deprovision_op1",
					},
					http.StatusOK,
				)
			})

			JustBeforeEach(func() {
				deprovisionResp, deprovisionErr = brokerClient.Deprovision(ctx, osbapi.DeprovisionPayload{
					ID: "my-service-instance",
					DeprovisionRequestParamaters: osbapi.DeprovisionRequestParamaters{
						ServiceId: "service-guid",
						PlanID:    "plan-guid",
					},
				})
			})

			It("deprovisions the service synchronously", func() {
				Expect(deprovisionErr).NotTo(HaveOccurred())
				Expect(deprovisionResp).To(Equal(osbapi.ProvisionResponse{
					IsAsync:   false,
					Operation: "deprovision_op1",
				}))
			})

			It("sends async deprovision request to broker", func() {
				Expect(deprovisionErr).NotTo(HaveOccurred())
				requests := brokerServer.ServedRequests()

				Expect(requests).To(HaveLen(1))

				Expect(requests[0].Method).To(Equal(http.MethodDelete))
				Expect(requests[0].URL.Path).To(Equal("/v2/service_instances/my-service-instance"))

				Expect(requests[0].URL.Query()).To(BeEquivalentTo(map[string][]string{
					"service_id":         {"service-guid"},
					"plan_id":            {"plan-guid"},
					"accepts_incomplete": {"true"},
				}))
			})

			When("the broker accepts the deprovision request", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse(
						"/v2/service_instances/{id}",
						map[string]any{
							"operation": "deprovision_op1",
						},
						http.StatusAccepted,
					)
				})

				It("deprovisions the service asynchronously", func() {
					Expect(deprovisionErr).NotTo(HaveOccurred())
					Expect(deprovisionResp).To(Equal(osbapi.ProvisionResponse{
						IsAsync:   true,
						Operation: "deprovision_op1",
					}))
				})
			})

			When("the deprovision request fails with 400 BadRequest error", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse("/v2/service_instances/{id}", nil, http.StatusBadRequest)
				})

				It("returns an unrecoverable error", func() {
					Expect(deprovisionErr).To(Equal(osbapi.UnrecoverableError{Status: http.StatusBadRequest}))
				})
			})

			When("the deprovision request fails with 410 Gone error", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse("/v2/service_instances/{id}", nil, http.StatusGone)
				})

				It("returns a Gone error", func() {
					Expect(deprovisionErr).To(Equal(osbapi.GoneError{}))
				})
			})

			When("the provision request fails with 422 Unprocessable entity error", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse("/v2/service_instances/{id}", nil, http.StatusUnprocessableEntity)
				})

				It("returns an unrecoverable error", func() {
					Expect(deprovisionErr).To(Equal(osbapi.UnrecoverableError{Status: http.StatusUnprocessableEntity}))
				})
			})

			When("the deprovision request fails", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse(
						"/v2/service_instances/{id}",
						nil,
						http.StatusTeapot,
					)
				})

				It("returns an error", func() {
					Expect(deprovisionErr).To(MatchError(ContainSubstring("deprovision request failed")))
				})
			})
		})

		Describe("GetServiceInstanceLastOperation", func() {
			var (
				lastOpResp           osbapi.LastOperationResponse
				lastOpErr            error
				lastOperationRequest osbapi.GetInstanceLastOperationRequest
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

				lastOperationRequest = osbapi.GetInstanceLastOperationRequest{
					InstanceID: "my-service-instance",
					GetLastOperationRequestParameters: osbapi.GetLastOperationRequestParameters{
						ServiceId: "service-guid",
						PlanID:    "plan-guid",
						Operation: "op-guid",
					},
				}
			})

			JustBeforeEach(func() {
				lastOpResp, lastOpErr = brokerClient.GetServiceInstanceLastOperation(ctx, lastOperationRequest)
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
				Expect(requests[0].URL.Query()).To(BeEquivalentTo(map[string][]string{
					"service_id": {"service-guid"},
					"plan_id":    {"plan-guid"},
					"operation":  {"op-guid"},
				}))

				requestBytes, err := io.ReadAll(requests[0].Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(requestBytes).To(BeEmpty())
			})

			When("request parameters are not specified", func() {
				BeforeEach(func() {
					lastOperationRequest = osbapi.GetInstanceLastOperationRequest{
						InstanceID: "my-service-instance",
					}
				})

				It("does not specify http request query parameters", func() {
					Expect(lastOpErr).NotTo(HaveOccurred())
					requests := brokerServer.ServedRequests()

					Expect(requests).To(HaveLen(1))
					Expect(requests[0].URL.Query()).To(BeEmpty())
				})
			})

			When("getting the last operation request fails", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse(
						"/v2/service_instances/{id}/last_operation",
						nil,
						http.StatusTeapot,
					)
				})

				It("returns an error", func() {
					Expect(lastOpErr).To(MatchError(ContainSubstring("last operation request failed")))
				})
			})

			When("getting the last operation request fails with 410 Gone", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse(
						"/v2/service_instances/{id}/last_operation",
						nil,
						http.StatusGone,
					)
				})

				It("returns a gone error", func() {
					Expect(lastOpErr).To(BeAssignableToTypeOf(osbapi.GoneError{}))
				})
			})
		})
	})

	Describe("Bindings", func() {
		Describe("Bind", func() {
			var (
				bindResp osbapi.BindResponse
				bindErr  error
			)

			BeforeEach(func() {
				brokerServer.WithResponse(
					"/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
					map[string]any{
						"credentials": map[string]string{
							"foo": "bar",
						},
					},
					http.StatusCreated,
				)
			})

			JustBeforeEach(func() {
				bindResp, bindErr = brokerClient.Bind(ctx, osbapi.BindPayload{
					InstanceID: "instance-id",
					BindingID:  "binding-id",
					BindRequest: osbapi.BindRequest{
						ServiceId: "service-guid",
						PlanID:    "plan-guid",
						AppGUID:   "app-guid",
						BindResource: osbapi.BindResource{
							AppGUID: "app-guid",
						},
						Parameters: map[string]any{
							"foo": "bar",
						},
					},
				})
			})

			It("sends async bind request to broker", func() {
				Expect(bindErr).NotTo(HaveOccurred())
				requests := brokerServer.ServedRequests()

				Expect(requests).To(HaveLen(1))

				Expect(requests[0].Method).To(Equal(http.MethodPut))
				Expect(requests[0].URL.Path).To(Equal("/v2/service_instances/instance-id/service_bindings/binding-id"))

				Expect(requests[0].URL.Query().Get("accepts_incomplete")).To(Equal("true"))
			})

			It("sends correct request to broker", func() {
				Expect(bindErr).NotTo(HaveOccurred())
				requests := brokerServer.ServedRequests()

				Expect(requests).To(HaveLen(1))

				Expect(requests[0].Method).To(Equal(http.MethodPut))
				Expect(requests[0].URL.Path).To(Equal("/v2/service_instances/instance-id/service_bindings/binding-id"))

				requestBytes, err := io.ReadAll(requests[0].Body)
				Expect(err).NotTo(HaveOccurred())
				requestBody := map[string]any{}
				Expect(json.Unmarshal(requestBytes, &requestBody)).To(Succeed())

				Expect(requestBody).To(MatchAllKeys(Keys{
					"service_id": Equal("service-guid"),
					"plan_id":    Equal("plan-guid"),
					"app_guid":   Equal("app-guid"),
					"bind_resource": MatchAllKeys(Keys{
						"app_guid": Equal("app-guid"),
					}),
					"parameters": MatchAllKeys(Keys{
						"foo": Equal("bar"),
					}),
				}))
			})

			It("binds the service", func() {
				Expect(bindErr).NotTo(HaveOccurred())
				Expect(bindResp).To(Equal(osbapi.BindResponse{
					Credentials: map[string]any{
						"foo": "bar",
					},
				}))
			})

			When("bind is asynchronous", func() {
				BeforeEach(func() {
					brokerServer.WithResponse(
						"/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
						map[string]any{
							"operation": "bind_op1",
						},
						http.StatusAccepted,
					)
				})

				It("binds the service asynchronously", func() {
					Expect(bindErr).NotTo(HaveOccurred())
					Expect(bindResp).To(Equal(osbapi.BindResponse{
						Operation: "bind_op1",
						IsAsync:   true,
					}))
				})
			})

			When("binding request fails", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse(
						"/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
						nil,
						http.StatusTeapot,
					)
				})

				It("returns an error", func() {
					Expect(bindErr).To(MatchError(ContainSubstring("binding request failed")))
				})
			})

			When("binding request fails with 409 Conflict", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse(
						"/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
						nil,
						http.StatusConflict,
					)
				})

				It("returns an unrecoverable error", func() {
					Expect(bindErr).To(BeAssignableToTypeOf(osbapi.UnrecoverableError{}))
				})
			})

			When("binding request fails with 422 Unprocessable Entity", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse(
						"/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
						nil,
						http.StatusUnprocessableEntity,
					)
				})

				It("returns an unrecoverable error", func() {
					Expect(bindErr).To(BeAssignableToTypeOf(osbapi.UnrecoverableError{}))
				})
			})

			When("binding request fails with 400 Bad Request", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse(
						"/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
						nil,
						http.StatusBadRequest,
					)
				})

				It("returns an unrecoverable error", func() {
					Expect(bindErr).To(BeAssignableToTypeOf(osbapi.UnrecoverableError{}))
				})
			})
		})

		Describe("GetServiceBinding", func() {
			var (
				bindingResp osbapi.BindingResponse
				getBindErr  error
			)
			BeforeEach(func() {
				brokerServer.WithResponse(
					"/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
					map[string]any{
						"parameters": map[string]string{
							"billing-account": "abcde12345",
						},
					},
					http.StatusOK,
				)
			})
			JustBeforeEach(func() {
				bindingResp, getBindErr = brokerClient.GetServiceBinding(ctx, osbapi.BindPayload{
					InstanceID: "my-service-instance",
					BindingID:  "my-binding-id",
					BindRequest: osbapi.BindRequest{
						ServiceId: "my-service-offering-id",
						PlanID:    "my-plan-id",
					},
				})
			})

			It("gets the service binding", func() {
				Expect(getBindErr).NotTo(HaveOccurred())

				requests := brokerServer.ServedRequests()
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Method).To(Equal(http.MethodGet))
				Expect(requests[0].URL.Path).To(Equal("/v2/service_instances/my-service-instance/service_bindings/my-binding-id"))
				Expect(requests[0].URL.Query()).To(BeEquivalentTo(map[string][]string{
					"service_id": {"my-service-offering-id"},
					"plan_id":    {"my-plan-id"},
				}))

				Expect(bindingResp).To(Equal(osbapi.BindingResponse{
					Parameters: map[string]any{
						"billing-account": "abcde12345",
					},
				}))
			})

			When("the service binding does not exist", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse(
						"/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
						nil,
						http.StatusNotFound,
					)
				})

				It("returns an error", func() {
					Expect(getBindErr).To(MatchError(ContainSubstring(fmt.Sprintf("The server responded with status: %d", http.StatusNotFound))))
				})
			})
		})

		Describe("GetServiceBindingLastOperation", func() {
			var (
				lastOpResp osbapi.LastOperationResponse
				lastOpErr  error
			)

			BeforeEach(func() {
				brokerServer.WithResponse(
					"/v2/service_instances/{instance_id}/service_bindings/{binding_id}/last_operation",
					map[string]any{
						"state":       "in-progress",
						"description": "provisioning",
					},
					http.StatusOK,
				)
			})

			JustBeforeEach(func() {
				lastOpResp, lastOpErr = brokerClient.GetServiceBindingLastOperation(ctx, osbapi.GetBindingLastOperationRequest{
					InstanceID: "my-service-instance",
					BindingID:  "my-binding-id",
					GetLastOperationRequestParameters: osbapi.GetLastOperationRequestParameters{
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
				Expect(requests[0].URL.Path).To(Equal("/v2/service_instances/my-service-instance/service_bindings/my-binding-id/last_operation"))

				requestBytes, err := io.ReadAll(requests[0].Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(requestBytes).To(BeEmpty())
			})

			When("getting the last operation request fails", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse(
						"/v2/service_instances/{instance_id}/service_bindings/{binding_id}/last_operation",
						nil,
						http.StatusTeapot,
					)
				})

				It("returns an error", func() {
					Expect(lastOpErr).To(MatchError(ContainSubstring("last operation request failed")))
				})
			})

			When("getting the last operation request fails with 410 Gone", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse(
						"/v2/service_instances/{instance_id}/service_bindings/{binding_id}/last_operation",
						nil,
						http.StatusGone,
					)
				})

				It("returns a gone error", func() {
					Expect(lastOpErr).To(BeAssignableToTypeOf(osbapi.GoneError{}))
				})
			})
		})

		Describe("Unbind", func() {
			var (
				unbindResp osbapi.UnbindResponse
				unbindErr  error
			)

			BeforeEach(func() {
				brokerServer.WithResponse(
					"/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
					nil,
					http.StatusOK,
				)
			})

			JustBeforeEach(func() {
				unbindResp, unbindErr = brokerClient.Unbind(ctx, osbapi.UnbindPayload{
					InstanceID: "instance-id",
					BindingID:  "binding-id",
					UnbindRequestParameters: osbapi.UnbindRequestParameters{
						ServiceId: "service-guid",
						PlanID:    "plan-guid",
					},
				})
			})

			It("sends an unbind request to broker", func() {
				Expect(unbindErr).NotTo(HaveOccurred())
				requests := brokerServer.ServedRequests()

				Expect(requests).To(HaveLen(1))

				Expect(requests[0].Method).To(Equal(http.MethodDelete))
				Expect(requests[0].URL.Path).To(Equal("/v2/service_instances/instance-id/service_bindings/binding-id"))

				Expect(requests[0].URL.Query()).To(BeEquivalentTo(map[string][]string{
					"service_id":         {"service-guid"},
					"plan_id":            {"plan-guid"},
					"accepts_incomplete": {"true"},
				}))
			})

			It("responds synchronously", func() {
				Expect(unbindErr).NotTo(HaveOccurred())
				Expect(unbindResp.IsComplete()).To(BeTrue())
			})

			When("the broker accepts the unbind request", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse(
						"/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
						map[string]any{
							"operation": "unbind_op1",
						},
						http.StatusAccepted,
					)
				})

				It("unbinds the service asynchronously", func() {
					Expect(unbindErr).NotTo(HaveOccurred())
					Expect(unbindResp).To(Equal(osbapi.UnbindResponse{
						IsAsync:   true,
						Operation: "unbind_op1",
					}))
				})
			})

			When("the unbind request fails", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse(
						"/v2/service_instances/{instance_id}/service_bindings/{binding_id}",
						nil,
						http.StatusTeapot,
					)
				})

				It("returns an error", func() {
					Expect(unbindErr).To(MatchError(ContainSubstring("unbind request failed")))
				})
			})

			When("the unbind request fails with 400 BadRequest error", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse("/v2/service_instances/{instance_id}/service_bindings/{binding_id}", nil, http.StatusBadRequest)
				})

				It("returns an unrecoverable error", func() {
					Expect(unbindErr).To(Equal(osbapi.UnrecoverableError{Status: http.StatusBadRequest}))
				})
			})

			When("the unbind request fails with 410 Gone error", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse("/v2/service_instances/{instance_id}/service_bindings/{binding_id}", nil, http.StatusGone)
				})

				It("returns a Gone error", func() {
					Expect(unbindErr).To(Equal(osbapi.GoneError{}))
				})
			})

			When("the unbind request fails with 422 Unprocessable entity error", func() {
				BeforeEach(func() {
					brokerServer = brokerServer.WithResponse("/v2/service_instances/{instance_id}/service_bindings/{binding_id}", nil, http.StatusUnprocessableEntity)
				})

				It("returns an unrecoverable error", func() {
					Expect(unbindErr).To(Equal(osbapi.UnrecoverableError{Status: http.StatusUnprocessableEntity}))
				})
			})
		})
	})
})
