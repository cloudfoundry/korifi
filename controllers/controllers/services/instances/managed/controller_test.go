package managed_test

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi/fake"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFServiceInstance", func() {
	var (
		brokerClient  *fake.BrokerClient
		instance      *korifiv1alpha1.CFServiceInstance
		serviceBroker *korifiv1alpha1.CFServiceBroker
		servicePlan   *korifiv1alpha1.CFServicePlan
	)

	BeforeEach(func() {
		brokerClient = new(fake.BrokerClient)
		brokerClientFactory.CreateClientReturns(brokerClient, nil)

		brokerClient.ProvisionReturns(osbapi.ProvisionResponse{}, nil)

		serviceBroker = &korifiv1alpha1.CFServiceBroker{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rootNamespace,
				Name:      uuid.NewString(),
			},
			Spec: korifiv1alpha1.CFServiceBrokerSpec{
				Name: "my-service-broker",
				Credentials: corev1.LocalObjectReference{
					Name: "my-broker-secret",
				},
			},
		}
		Expect(adminClient.Create(ctx, serviceBroker)).To(Succeed())

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: uuid.NewString(),
				Labels: map[string]string{
					korifiv1alpha1.SpaceGUIDKey: "space-guid",
					korifiv1alpha1.OrgGUIDKey:   "org-guid",
				},
			},
		}
		Expect(adminClient.Create(ctx, namespace)).To(Succeed())

		serviceOffering := &korifiv1alpha1.CFServiceOffering{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: rootNamespace,
				Labels: map[string]string{
					korifiv1alpha1.RelServiceBrokerGUIDLabel: serviceBroker.Name,
				},
			},
			Spec: korifiv1alpha1.CFServiceOfferingSpec{
				Name: "service-offering-name",
				BrokerCatalog: korifiv1alpha1.ServiceBrokerCatalog{
					ID: "service-offering-id",
				},
			},
		}
		Expect(adminClient.Create(ctx, serviceOffering)).To(Succeed())

		servicePlan = &korifiv1alpha1.CFServicePlan{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: rootNamespace,
				Labels: map[string]string{
					korifiv1alpha1.RelServiceBrokerGUIDLabel:   serviceBroker.Name,
					korifiv1alpha1.RelServiceOfferingGUIDLabel: serviceOffering.Name,
				},
			},
			Spec: korifiv1alpha1.CFServicePlanSpec{
				Visibility: korifiv1alpha1.ServicePlanVisibility{
					Type: "public",
				},
				BrokerCatalog: korifiv1alpha1.ServicePlanBrokerCatalog{
					ID: "service-plan-id",
				},
				MaintenanceInfo: korifiv1alpha1.MaintenanceInfo{
					Version: "1.2.3",
				},
			},
		}
		Expect(adminClient.Create(ctx, servicePlan)).To(Succeed())

		instance = &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: namespace.Name,
				Finalizers: []string{
					korifiv1alpha1.CFServiceInstanceFinalizerName,
				},
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "service-instance-name",
				Type:        korifiv1alpha1.ManagedType,
				PlanGUID:    servicePlan.Name,
			},
		}

		Expect(adminClient.Create(ctx, instance)).To(Succeed())
	})

	It("sets the ObservedGeneration status field", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
			g.Expect(instance.Status.ObservedGeneration).To(Equal(instance.Generation))
		}).Should(Succeed())
	})

	It("sets the Ready condition to True", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
			g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
				HasType(Equal(korifiv1alpha1.StatusConditionReady)),
				HasStatus(Equal(metav1.ConditionTrue)),
			)))
		}).Should(Succeed())
	})

	It("defaults the service label to the service offering name", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
			g.Expect(instance.Spec.ServiceLabel).To(PointTo(Equal("service-offering-name")))
		}).Should(Succeed())
	})

	It("sets the plan maintenance info in the status", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
			g.Expect(instance.Status.MaintenanceInfo).To(Equal(korifiv1alpha1.MaintenanceInfo{
				Version: "1.2.3",
			}))
		}).Should(Succeed())
	})

	It("sets upgrage available to false in the status", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
			g.Expect(instance.Status.UpgradeAvailable).To(BeFalse())
		}).Should(Succeed())
	})

	When("the service label is set", func() {
		BeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, instance, func() {
				instance.Spec.ServiceLabel = tools.PtrTo("custom-service-label")
			})).To(Succeed())
		})

		It("does not override it", func() {
			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(instance.Spec.ServiceLabel).To(PointTo(Equal("custom-service-label")))
			}).Should(Succeed())
		})
	})

	It("does not check last operation", func() {
		Consistently(func(g Gomega) {
			g.Expect(brokerClient.GetServiceInstanceLastOperationCallCount()).To(Equal(0))
		}).Should(Succeed())
	})

	It("provisions the service", func() {
		Eventually(func(g Gomega) {
			g.Expect(brokerClient.ProvisionCallCount()).NotTo(BeZero())
			_, payload := brokerClient.ProvisionArgsForCall(0)
			g.Expect(payload).To(Equal(osbapi.ProvisionPayload{
				InstanceID: instance.Name,
				ProvisionRequest: osbapi.ProvisionRequest{
					ServiceId: "service-offering-id",
					PlanID:    "service-plan-id",
					SpaceGUID: "space-guid",
					OrgGUID:   "org-guid",
				},
			}))
		}).Should(Succeed())
	})

	It("sets succeeded state in instance last operation", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
			g.Expect(brokerClient.ProvisionCallCount()).To(BeNumerically(">=", 1))
			g.Expect(instance.Status.LastOperation).To(Equal(korifiv1alpha1.LastOperation{
				Type:  "create",
				State: "succeeded",
			}))
		}).Should(Succeed())
	})

	When("the service instance parameters are not set", func() {
		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, instance, func() {
				instance.Spec.Parameters.Name = ""
			})).To(Succeed())
		})

		It("requests provisioning without parameters", func() {
			Eventually(func(g Gomega) {
				g.Expect(brokerClient.ProvisionCallCount()).NotTo(BeZero())
				_, payload := brokerClient.ProvisionArgsForCall(0)
				g.Expect(payload.Parameters).To(BeEmpty())
			}).Should(Succeed())
		})
	})

	When("the service plan does not exist", func() {
		BeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, instance, func() {
				instance.Spec.PlanGUID = "i-do-not-exist"
			})).To(Succeed())
		})

		It("sets the ready condition to false", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

				g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
				)))
			}).Should(Succeed())
		})
	})

	When("the service broker does not exist", func() {
		BeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, servicePlan, func() {
				servicePlan.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel] = "invalid"
			})).To(Succeed())
		})

		It("sets the ready condition to false", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

				g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
				)))
			}).Should(Succeed())
		})
	})

	When("the service offering does not exist", func() {
		BeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, servicePlan, func() {
				servicePlan.Labels[korifiv1alpha1.RelServiceOfferingGUIDLabel] = "invalid"
			})).To(Succeed())
		})

		It("sets the ready condition to false", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

				g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
				)))
			}).Should(Succeed())
		})
	})

	When("the instance has parameters", func() {
		var paramsSecret *corev1.Secret

		BeforeEach(func() {
			paramsSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: instance.Namespace,
					Name:      uuid.NewString(),
				},
				Data: map[string][]byte{
					tools.ParametersSecretKey: []byte(`{"p1":"p1-value"}`),
				},
			}
			Expect(adminClient.Create(ctx, paramsSecret)).To(Succeed())

			Expect(k8s.Patch(ctx, adminClient, instance, func() {
				instance.Spec.Parameters.Name = paramsSecret.Name
			})).To(Succeed())
		})

		It("sends them to the broker on provision", func() {
			Eventually(func(g Gomega) {
				g.Expect(brokerClient.ProvisionCallCount()).To(BeNumerically(">", 0))
				_, payload := brokerClient.ProvisionArgsForCall(0)
				g.Expect(payload.Parameters).To(Equal(map[string]any{
					"p1": "p1-value",
				}))
			}).Should(Succeed())
		})

		When("the parameters secret does not exist", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, instance, func() {
					instance.Spec.Parameters.Name = "not-valid"
				})).To(Succeed())
			})

			It("sets the ready condition to false", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasReason(Equal("InvalidParameters")),
					)))
				}).Should(Succeed())
			})
		})
	})

	When("the provisioning is asynchronous", func() {
		BeforeEach(func() {
			brokerClient.GetServiceInstanceLastOperationReturns(osbapi.LastOperationResponse{
				State: "in-progress-or-whatever",
			}, nil)

			brokerClient.ProvisionReturns(osbapi.ProvisionResponse{
				IsAsync:   true,
				Operation: "operation-1",
			}, nil)
		})

		It("set sets ready condition to false", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

				g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
					HasReason(Equal("ProvisionInProgress")),
				)))
			}).Should(Succeed())
		})

		It("sets in progress state in instance last operation", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(brokerClient.ProvisionCallCount()).To(BeNumerically(">=", 1))
				g.Expect(instance.Status.LastOperation).To(Equal(korifiv1alpha1.LastOperation{
					Type:  "create",
					State: "in progress",
				}))
			}).Should(Succeed())
		})

		It("continuously checks the last operation", func() {
			Eventually(func(g Gomega) {
				g.Expect(brokerClient.GetServiceInstanceLastOperationCallCount()).To(BeNumerically(">", 1))
				_, lastOp := brokerClient.GetServiceInstanceLastOperationArgsForCall(brokerClient.GetServiceInstanceLastOperationCallCount() - 1)
				g.Expect(lastOp).To(Equal(osbapi.GetInstanceLastOperationRequest{
					InstanceID: instance.Name,
					GetLastOperationRequestParameters: osbapi.GetLastOperationRequestParameters{
						ServiceId: "service-offering-id",
						PlanID:    "service-plan-id",
						Operation: "operation-1",
					},
				}))
			}).Should(Succeed())
		})

		When("getting service last operation fails", func() {
			BeforeEach(func() {
				brokerClient.GetServiceInstanceLastOperationReturns(osbapi.LastOperationResponse{}, errors.New("get-last-op-failed"))
			})

			It("sets the ready condition to false", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

					g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
					)))
				}).Should(Succeed())
			})
		})

		When("the last operation is succeeded", func() {
			BeforeEach(func() {
				brokerClient.GetServiceInstanceLastOperationReturns(osbapi.LastOperationResponse{
					State: "succeeded",
				}, nil)
			})

			It("sets the ready condition to true", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

					g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionTrue)),
					)))
				}).Should(Succeed())
			})
		})

		When("the last operation is failed", func() {
			BeforeEach(func() {
				brokerClient.GetServiceInstanceLastOperationReturns(osbapi.LastOperationResponse{
					State:       "failed",
					Description: "provision-failed",
				}, nil)
			})

			It("sets the ready condition to false", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

					g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
					)))
				}).Should(Succeed())
			})

			It("sets the failed condition", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

					g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.ProvisioningFailedCondition)),
						HasStatus(Equal(metav1.ConditionTrue)),
						HasReason(Equal("ProvisionFailed")),
						HasMessage(Equal("provision-failed")),
					)))
				}).Should(Succeed())
			})

			It("sets failed state in instance last operation", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(brokerClient.ProvisionCallCount()).To(BeNumerically(">=", 1))
					g.Expect(instance.Status.LastOperation).To(Equal(korifiv1alpha1.LastOperation{
						Type:        "create",
						State:       "failed",
						Description: "provision-failed",
					}))
				}).Should(Succeed())
			})
		})
	})

	When("service provisioning fails with recoverable error", func() {
		BeforeEach(func() {
			brokerClient.ProvisionReturns(osbapi.ProvisionResponse{}, errors.New("provision-failed"))
		})

		It("keeps trying to provision the instance", func() {
			Eventually(func(g Gomega) {
				g.Expect(brokerClient.ProvisionCallCount()).To(BeNumerically(">", 1))
				_, provisionPayload := brokerClient.ProvisionArgsForCall(1)
				g.Expect(provisionPayload).To(Equal(osbapi.ProvisionPayload{
					InstanceID: instance.Name,
					ProvisionRequest: osbapi.ProvisionRequest{
						ServiceId: "service-offering-id",
						PlanID:    "service-plan-id",
						SpaceGUID: "space-guid",
						OrgGUID:   "org-guid",
					},
				}))
			}).Should(Succeed())
		})

		It("sets initial state in instance last operation", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(brokerClient.ProvisionCallCount()).To(BeNumerically(">=", 1))
				g.Expect(instance.Status.LastOperation).To(Equal(korifiv1alpha1.LastOperation{
					Type:  "create",
					State: "initial",
				}))
			}).Should(Succeed())
		})
	})

	When("service provisioning fails with unrecoverable error", func() {
		BeforeEach(func() {
			brokerClient.ProvisionReturns(osbapi.ProvisionResponse{}, osbapi.UnrecoverableError{Status: http.StatusBadRequest})
		})

		It("fails the instance", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

				g.Expect(instance.Status.Conditions).To(ContainElements(
					SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
					),
					SatisfyAll(
						HasType(Equal(korifiv1alpha1.ProvisioningFailedCondition)),
						HasStatus(Equal(metav1.ConditionTrue)),
						HasReason(Equal("ProvisionFailed")),
						HasMessage(ContainSubstring("The server responded with status: 400")),
					),
				))
			}).Should(Succeed())
		})

		It("sets failed state in instance last operation", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(brokerClient.ProvisionCallCount()).To(BeNumerically(">=", 1))
				g.Expect(instance.Status.LastOperation).To(Equal(korifiv1alpha1.LastOperation{
					Type:  "create",
					State: "failed",
				}))
			}).Should(Succeed())
		})
	})

	When("the instance has become ready", func() {
		BeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, instance, func() {
				instance.Status.MaintenanceInfo.Version = "1.2.3"
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:   korifiv1alpha1.StatusConditionReady,
					Status: metav1.ConditionTrue,
					Reason: "ready",
				})
			})).To(Succeed())
		})

		It("does not request provision", func() {
			Consistently(func(g Gomega) {
				g.Expect(brokerClient.ProvisionCallCount()).To(Equal(0))
			}).Should(Succeed())
		})

		It("does not check last operation", func() {
			Consistently(func(g Gomega) {
				g.Expect(brokerClient.GetServiceInstanceLastOperationCallCount()).To(Equal(0))
			}).Should(Succeed())
		})

		It("remains ready", func() {
			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, korifiv1alpha1.StatusConditionReady)).To(BeTrue())
			}).Should(Succeed())
		})

		When("the service plan becomes disabled after provisioning", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, servicePlan, func() {
					servicePlan.Spec.Visibility = korifiv1alpha1.ServicePlanVisibility{
						Type: "admin",
					}
				})).To(Succeed())
			})

			It("remains ready", func() {
				Consistently(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(meta.IsStatusConditionTrue(instance.Status.Conditions, korifiv1alpha1.StatusConditionReady)).To(BeTrue())
				}).Should(Succeed())
			})
		})

		When("the service plan has a new version", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, servicePlan, func() {
					servicePlan.Spec.MaintenanceInfo.Version = "2.3.4"
				})).To(Succeed())
			})

			It("preserves the maintenance info", func() {
				Consistently(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(instance.Status.MaintenanceInfo.Version).To(Equal("1.2.3"))
				}).Should(Succeed())
			})

			It("sets upgradeAvailable to true", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(instance.Status.UpgradeAvailable).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})

	When("the instance provisioning has failed", func() {
		BeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, instance, func() {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:   korifiv1alpha1.ProvisioningFailedCondition,
					Status: metav1.ConditionTrue,
					Reason: "ProvisionFailed",
				})
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:   korifiv1alpha1.StatusConditionReady,
					Status: metav1.ConditionFalse,
					Reason: "ProvisionFailed",
				})
			})).To(Succeed())
		})

		It("does not request provision", func() {
			Consistently(func(g Gomega) {
				g.Expect(brokerClient.ProvisionCallCount()).To(Equal(0))
			}).Should(Succeed())
		})

		It("does not check last operation", func() {
			Consistently(func(g Gomega) {
				g.Expect(brokerClient.GetServiceInstanceLastOperationCallCount()).To(Equal(0))
			}).Should(Succeed())
		})

		It("remains not ready", func() {
			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
				)))
			}).Should(Succeed())
		})

		It("remains failed", func() {
			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.ProvisioningFailedCondition)),
					HasStatus(Equal(metav1.ConditionTrue)),
				)))
			}).Should(Succeed())
		})
	})

	When("the service plan has admin visibility type", func() {
		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, servicePlan, func() {
				servicePlan.Spec.Visibility = korifiv1alpha1.ServicePlanVisibility{
					Type: korifiv1alpha1.AdminServicePlanVisibilityType,
				}
			})).To(Succeed())
		})

		It("fails the instance", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

				g.Expect(instance.Status.Conditions).To(ContainElements(
					SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasReason(Equal("InvalidServicePlan")),
						HasMessage(Equal("The service plan is disabled")),
					),
				))
			}).Should(Succeed())
		})

		When("the plan eventually becomes visible", func() {
			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(instance.Status.Conditions).To(ContainElements(
						SatisfyAll(
							HasType(Equal(korifiv1alpha1.StatusConditionReady)),
							HasStatus(Equal(metav1.ConditionFalse)),
							HasReason(Equal("InvalidServicePlan")),
						),
					))
				}).Should(Succeed())

				Expect(k8s.PatchResource(ctx, adminClient, servicePlan, func() {
					servicePlan.Spec.Visibility = korifiv1alpha1.ServicePlanVisibility{
						Type: korifiv1alpha1.PublicServicePlanVisibilityType,
					}
				})).To(Succeed())
			})

			It("becomes ready", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionTrue)),
					)))
				}).Should(Succeed())
			})
		})
	})

	When("the service plan has org visibility type", func() {
		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, servicePlan, func() {
				servicePlan.Spec.Visibility = korifiv1alpha1.ServicePlanVisibility{
					Type: korifiv1alpha1.OrganizationServicePlanVisibilityType,
				}
			})).To(Succeed())
		})

		It("fails the instance", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

				g.Expect(instance.Status.Conditions).To(ContainElements(
					SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasReason(Equal("InvalidServicePlan")),
						HasMessage(Equal("The service plan is disabled")),
					),
				))
			}).Should(Succeed())
		})

		When("the instance org is allowed on the plan", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, servicePlan, func() {
					servicePlan.Spec.Visibility.Organizations = []string{
						"org-guid",
					}
				})).To(Succeed())
			})

			It("becomes ready", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionTrue)),
					)))
				}).Should(Succeed())
			})
		})
	})

	When("the service instance is purged", func() {
		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, instance, func() {
				controllerutil.RemoveFinalizer(instance, korifiv1alpha1.CFServiceInstanceFinalizerName)
			})).To(Succeed())
		})

		JustBeforeEach(func() {
			Expect(k8sManager.GetClient().Delete(ctx, instance)).To(Succeed())
		})

		It("does not contact the broker for deprovisioning", func() {
			Eventually(func(g Gomega) {
				err := adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())

				g.Expect(brokerClient.DeprovisionCallCount()).To(Equal(0))
			}).Should(Succeed())
		})
	})

	When("the service instance is user-provided", func() {
		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, instance, func() {
				instance.Spec.Type = korifiv1alpha1.UserProvidedType
			})).To(Succeed())
		})

		It("does not reconcile it", func() {
			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(instance.Status).To(BeZero())
			}).Should(Succeed())
		})
	})

	Describe("instance deletion", func() {
		BeforeEach(func() {
			brokerClient.DeprovisionReturns(osbapi.ProvisionResponse{
				Operation: "deprovision-op",
			}, nil)
		})

		JustBeforeEach(func() {
			// For deletion test we want to request deletion and verify the behaviour when finalization fails.
			// Therefore we use the standard k8s client instncea of `adminClient` as it ensures that the object is deleted
			Expect(k8sManager.GetClient().Delete(ctx, instance)).To(Succeed())
		})

		It("deprovisions the instance with the broker", func() {
			Eventually(func(g Gomega) {
				err := adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())

				g.Expect(brokerClient.DeprovisionCallCount()).To(Equal(1))
				_, actualDeprovisionRequest := brokerClient.DeprovisionArgsForCall(0)
				Expect(actualDeprovisionRequest).To(Equal(osbapi.DeprovisionPayload{
					ID: instance.Name,
					DeprovisionRequestParamaters: osbapi.DeprovisionRequestParamaters{
						ServiceId: "service-offering-id",
						PlanID:    "service-plan-id",
					},
				}))
			}).Should(Succeed())
		})

		When("deprovision fails", func() {
			BeforeEach(func() {
				brokerClient.DeprovisionReturns(osbapi.ProvisionResponse{}, errors.New("deprovision-failed"))
			})

			It("the instance is not deleted", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(brokerClient.DeprovisionCallCount()).To(BeNumerically(">", 1))
				}).Should(Succeed())
			})
		})

		When("the instance is deleted asynchronously", func() {
			BeforeEach(func() {
				brokerClient.DeprovisionReturns(osbapi.ProvisionResponse{
					IsAsync:   true,
					Operation: "deprovision-op",
				}, nil)
			})

			When("the last operation is in progress", func() {
				BeforeEach(func() {
					brokerClient.GetServiceInstanceLastOperationReturns(osbapi.LastOperationResponse{
						State: "in progress",
					}, nil)
				})

				It("sets the ready condition to false", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

						g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
							HasType(Equal(korifiv1alpha1.StatusConditionReady)),
							HasStatus(Equal(metav1.ConditionFalse)),
						)))
					}).Should(Succeed())
				})

				It("sets in progress state in instance last operation", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
						g.Expect(brokerClient.DeprovisionCallCount()).To(BeNumerically(">", 1))
						g.Expect(instance.Status.LastOperation).To(Equal(korifiv1alpha1.LastOperation{
							Type:  "delete",
							State: "in progress",
						}))
					}).Should(Succeed())
				})

				It("keeps checking last operation", func() {
					Eventually(func(g Gomega) {
						g.Expect(brokerClient.GetServiceInstanceLastOperationCallCount()).To(BeNumerically(">", 1))
						_, actualLastOpPayload := brokerClient.GetServiceInstanceLastOperationArgsForCall(1)
						g.Expect(actualLastOpPayload).To(Equal(osbapi.GetInstanceLastOperationRequest{
							InstanceID: instance.Name,
							GetLastOperationRequestParameters: osbapi.GetLastOperationRequestParameters{
								ServiceId: "service-offering-id",
								PlanID:    "service-plan-id",
								Operation: "deprovision-op",
							},
						}))
					}).Should(Succeed())
				})
			})

			When("the last operation is failed", func() {
				BeforeEach(func() {
					brokerClient.GetServiceInstanceLastOperationReturns(osbapi.LastOperationResponse{
						State: "failed",
					}, nil)
				})

				It("sets the failed condition", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

						g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
							HasType(Equal(korifiv1alpha1.StatusConditionReady)),
							HasStatus(Equal(metav1.ConditionFalse)),
						)))

						g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
							HasType(Equal(korifiv1alpha1.DeprovisioningFailedCondition)),
							HasStatus(Equal(metav1.ConditionTrue)),
						)))
					}).Should(Succeed())
				})

				It("sets failed state in instance last operation", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
						g.Expect(brokerClient.DeprovisionCallCount()).To(BeNumerically(">", 1))
						g.Expect(instance.Status.LastOperation).To(Equal(korifiv1alpha1.LastOperation{
							Type:  "delete",
							State: "failed",
						}))
					}).Should(Succeed())
				})
			})

			When("service deprovisioning fails with recoverable error", func() {
				BeforeEach(func() {
					brokerClient.DeprovisionReturns(osbapi.ProvisionResponse{}, errors.New("deprovision-failed"))
				})

				It("keeps trying to deprovision the instance", func() {
					Eventually(func(g Gomega) {
						g.Expect(brokerClient.DeprovisionCallCount()).To(BeNumerically(">", 1))
						_, actualDeprovisionRequest := brokerClient.DeprovisionArgsForCall(0)
						g.Expect(actualDeprovisionRequest).To(Equal(osbapi.DeprovisionPayload{
							ID: instance.Name,
							DeprovisionRequestParamaters: osbapi.DeprovisionRequestParamaters{
								ServiceId: "service-offering-id",
								PlanID:    "service-plan-id",
							},
						}))
					}).Should(Succeed())
				})

				It("sets initial state in instance last operation", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
						g.Expect(brokerClient.DeprovisionCallCount()).To(BeNumerically(">=", 1))
						g.Expect(instance.Status.LastOperation).To(Equal(korifiv1alpha1.LastOperation{
							Type:  "delete",
							State: "initial",
						}))
					}).Should(Succeed())
				})
			})

			When("service deprovisioning fails with unrecoverable error", func() {
				BeforeEach(func() {
					brokerClient.DeprovisionReturns(osbapi.ProvisionResponse{}, osbapi.UnrecoverableError{Status: http.StatusUnprocessableEntity})
				})

				It("fails the instance", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

						g.Expect(instance.Status.Conditions).To(ContainElements(
							SatisfyAll(
								HasType(Equal(korifiv1alpha1.StatusConditionReady)),
								HasStatus(Equal(metav1.ConditionFalse)),
							),
							SatisfyAll(
								HasType(Equal(korifiv1alpha1.DeprovisioningFailedCondition)),
								HasStatus(Equal(metav1.ConditionTrue)),
								HasReason(Equal("DeprovisionFailed")),
								HasMessage(ContainSubstring("The server responded with status: 422")),
							),
						))
					}).Should(Succeed())
				})

				It("sets failed state in instance last operation", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
						g.Expect(brokerClient.DeprovisionCallCount()).To(BeNumerically(">=", 1))
						g.Expect(instance.Status.LastOperation).To(Equal(korifiv1alpha1.LastOperation{
							Type:  "delete",
							State: "failed",
						}))
					}).Should(Succeed())
				})
			})

			When("the instance is gone", func() {
				BeforeEach(func() {
					brokerClient.DeprovisionReturns(osbapi.ProvisionResponse{}, osbapi.GoneError{})
				})

				It("deletes the instance", func() {
					Eventually(func(g Gomega) {
						err := adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
						g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
					}).Should(Succeed())
				})
			})
		})
	})
})
