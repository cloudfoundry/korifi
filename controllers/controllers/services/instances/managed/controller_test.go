package managed_test

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/instances/managed/fake"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/model/services"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

		brokerClient.ProvisionReturns(osbapi.ServiceInstanceOperationResponse{
			Operation: "operation-1",
		}, nil)

		brokerClient.GetServiceInstanceLastOperationReturns(osbapi.LastOperationResponse{
			State: "succeeded",
		}, nil)

		serviceBroker = &korifiv1alpha1.CFServiceBroker{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rootNamespace,
				Name:      uuid.NewString(),
			},
			Spec: korifiv1alpha1.CFServiceBrokerSpec{
				ServiceBroker: services.ServiceBroker{
					Name: "my-service-broker",
				},
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
				ServiceOffering: services.ServiceOffering{
					BrokerCatalog: services.ServiceBrokerCatalog{
						ID: "service-offering-id",
					},
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
				ServicePlan: services.ServicePlan{
					BrokerCatalog: services.ServicePlanBrokerCatalog{
						ID: "service-plan-id",
					},
				},
			},
		}
		Expect(adminClient.Create(ctx, servicePlan)).To(Succeed())

		params := map[string]string{
			"param-key": "param-value",
		}
		paramBytes, err := json.Marshal(params)
		Expect(err).NotTo(HaveOccurred())

		instance = &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: namespace.Name,
				Finalizers: []string{
					korifiv1alpha1.CFManagedServiceInstanceFinalizerName,
				},
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "service-instance-name",
				Type:        korifiv1alpha1.ManagedType,
				PlanGUID:    servicePlan.Name,
				Parameters: &runtime.RawExtension{
					Raw: paramBytes,
				},
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

	It("provisions the service", func() {
		Eventually(func(g Gomega) {
			g.Expect(brokerClient.ProvisionCallCount()).To(Equal(1))
			_, payload := brokerClient.ProvisionArgsForCall(0)
			g.Expect(payload).To(Equal(osbapi.InstanceProvisionPayload{
				InstanceID: instance.Name,
				InstanceProvisionRequest: osbapi.InstanceProvisionRequest{
					ServiceId: "service-offering-id",
					PlanID:    "service-plan-id",
					SpaceGUID: "space-guid",
					OrgGUID:   "org-guid",
					Parameters: map[string]any{
						"param-key": "param-value",
					},
				},
			}))

			g.Expect(brokerClient.GetServiceInstanceLastOperationCallCount()).To(BeNumerically(">", 0))
			_, lastOp := brokerClient.GetServiceInstanceLastOperationArgsForCall(brokerClient.GetServiceInstanceLastOperationCallCount() - 1)
			g.Expect(lastOp).To(Equal(osbapi.GetLastOperationPayload{
				ID: instance.Name,
				GetLastOperationRequest: osbapi.GetLastOperationRequest{
					ServiceId: "service-offering-id",
					PlanID:    "service-plan-id",
					Operation: "operation-1",
				},
			}))
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
			g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
				HasType(Equal(korifiv1alpha1.ProvisionRequestedCondition)),
				HasStatus(Equal(metav1.ConditionTrue)),
			)))
			g.Expect(instance.Status.ProvisionOperation).To(Equal("operation-1"))
		}).Should(Succeed())
	})

	When("the service instance parameters are not set", func() {
		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, instance, func() {
				instance.Spec.Parameters = nil
			})).To(Succeed())
		})

		It("requests provisioning without parameters", func() {
			Eventually(func(g Gomega) {
				g.Expect(brokerClient.ProvisionCallCount()).To(Equal(1))
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

	When("the provisioning is synchronous", func() {
		BeforeEach(func() {
			brokerClient.ProvisionReturns(osbapi.ServiceInstanceOperationResponse{
				Operation: "operation-1",
				Complete:  true,
			}, nil)
		})

		It("does not check last operation", func() {
			Consistently(func(g Gomega) {
				g.Expect(brokerClient.GetServiceInstanceLastOperationCallCount()).To(Equal(0))
			}).Should(Succeed())
		})

		It("set sets ready condition to true", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())

				g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionTrue)),
				)))
			}).Should(Succeed())
		})
	})

	When("service provisioning fails", func() {
		BeforeEach(func() {
			brokerClient.ProvisionReturns(osbapi.ServiceInstanceOperationResponse{}, errors.New("provision-failed"))
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
						HasMessage(ContainSubstring("provision-failed")),
					),
				))
			}).Should(Succeed())
		})
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

		It("keeps checking last operation", func() {
			Eventually(func(g Gomega) {
				g.Expect(brokerClient.GetServiceInstanceLastOperationCallCount()).To(BeNumerically(">", 1))
				_, actualLastOpPayload := brokerClient.GetServiceInstanceLastOperationArgsForCall(1)
				g.Expect(actualLastOpPayload).To(Equal(osbapi.GetLastOperationPayload{
					ID: instance.Name,
					GetLastOperationRequest: osbapi.GetLastOperationRequest{
						ServiceId: "service-offering-id",
						PlanID:    "service-plan-id",
						Operation: "operation-1",
					},
				}))
			}).Should(Succeed())
		})

		It("sets the ProvisionRequested condition", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.ProvisionRequestedCondition)),
					HasStatus(Equal(metav1.ConditionTrue)),
				)))
			}).Should(Succeed())

			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.ProvisionRequestedCondition)),
					HasStatus(Equal(metav1.ConditionTrue)),
				)))
			}).Should(Succeed())
		})
	})

	When("the last operation is failed", func() {
		BeforeEach(func() {
			brokerClient.GetServiceInstanceLastOperationReturns(osbapi.LastOperationResponse{
				State: "failed",
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
				)))
			}).Should(Succeed())
		})
	})

	When("the instance has become ready", func() {
		BeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, instance, func() {
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
				g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionTrue)),
				)))
			}).Should(Succeed())
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

	When("the provisioninig has been requested", func() {
		BeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, instance, func() {
				meta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
					Type:   korifiv1alpha1.ProvisionRequestedCondition,
					Status: metav1.ConditionTrue,
					Reason: "ProvisionRequested",
				})
				instance.Status.ProvisionOperation = "operation-1"
			})).To(Succeed())
		})

		It("does not request provisioning again", func() {
			Consistently(func(g Gomega) {
				g.Expect(brokerClient.ProvisionCallCount()).To(Equal(0))
			}).Should(Succeed())
		})

		It("checks the provisioning status", func() {
			Eventually(func(g Gomega) {
				g.Expect(brokerClient.GetServiceInstanceLastOperationCallCount()).To(Equal(1))
				_, actualLastOpPayload := brokerClient.GetServiceInstanceLastOperationArgsForCall(0)
				g.Expect(actualLastOpPayload).To(Equal(osbapi.GetLastOperationPayload{
					ID: instance.Name,
					GetLastOperationRequest: osbapi.GetLastOperationRequest{
						ServiceId: "service-offering-id",
						PlanID:    "service-plan-id",
						Operation: "operation-1",
					},
				}))
			}).Should(Succeed())

			Consistently(func(g Gomega) {
				g.Expect(brokerClient.GetServiceInstanceLastOperationCallCount()).To(Equal(1))
			}).Should(Succeed())
		})
	})

	When("the instance is deleted", func() {
		BeforeEach(func() {
			brokerClient.DeprovisionReturns(osbapi.ServiceInstanceOperationResponse{
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
				Expect(actualDeprovisionRequest).To(Equal(osbapi.InstanceDeprovisionPayload{
					ID: instance.Name,
					InstanceDeprovisionRequest: osbapi.InstanceDeprovisionRequest{
						ServiceId: "service-offering-id",
						PlanID:    "service-plan-id",
					},
				}))
			}).Should(Succeed())
		})

		When("deprovision fails", func() {
			BeforeEach(func() {
				brokerClient.DeprovisionReturns(osbapi.ServiceInstanceOperationResponse{}, errors.New("deprovision-failed"))
			})

			It("sets ready condition to false", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasReason(Equal("DeprovisionFailed")),
					)))
				}).Should(Succeed())
			})
		})

		When("the service plan is not available", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, instance, func() {
					instance.Spec.PlanGUID = "does-not-exist"
				})).To(Succeed())
			})

			It("deletes the instance", func() {
				Eventually(func(g Gomega) {
					err := adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
					g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				}).Should(Succeed())
			})
		})

		When("the service offering does not exist", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, servicePlan, func() {
					servicePlan.Labels[korifiv1alpha1.RelServiceOfferingGUIDLabel] = "does-not-exist"
				})).To(Succeed())
			})

			It("deletes the instance", func() {
				Eventually(func(g Gomega) {
					err := adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
					g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				}).Should(Succeed())
			})
		})

		When("the service broker does not exist", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, servicePlan, func() {
					servicePlan.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel] = "does-not-exist"
				})).To(Succeed())
			})

			It("deletes the instance", func() {
				Eventually(func(g Gomega) {
					err := adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
					g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				}).Should(Succeed())
			})
		})

		When("creating the broker client fails", func() {
			BeforeEach(func() {
				brokerClientFactory.CreateClientReturns(nil, errors.New("client-create-err"))
			})

			It("deletes the instance", func() {
				Eventually(func(g Gomega) {
					err := adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
					g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				}).Should(Succeed())
			})
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
})
