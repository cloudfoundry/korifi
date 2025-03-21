package bindings_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/credentials"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi/fake"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "code.cloudfoundry.org/korifi/tests/matchers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFServiceBinding", func() {
	var (
		testNamespace string
		cfAppGUID     string
		instanceGUID  string
		binding       *korifiv1alpha1.CFServiceBinding
	)

	BeforeEach(func() {
		testNamespace = uuid.NewString()
		Expect(adminClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		})).To(Succeed())

		cfAppGUID = uuid.NewString()

		instanceGUID = uuid.NewString()

		binding = &korifiv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
				Finalizers: []string{
					korifiv1alpha1.CFServiceBindingFinalizerName,
				},
			},
			Spec: korifiv1alpha1.CFServiceBindingSpec{
				Service: corev1.ObjectReference{
					Kind:       "ServiceInstance",
					Name:       instanceGUID,
					APIVersion: "korifi.cloudfoundry.org/v1alpha1",
				},
				AppRef: corev1.LocalObjectReference{
					Name: cfAppGUID,
				},
				Type: korifiv1alpha1.CFServiceBindingTypeApp,
			},
		}
		Expect(adminClient.Create(ctx, binding)).To(Succeed())
	})

	Describe("user-provided bindings", func() {
		var (
			instance                  *korifiv1alpha1.CFServiceInstance
			instanceCredentialsSecret *corev1.Secret
		)

		BeforeEach(func() {
			credentialsBytes, err := json.Marshal(map[string]any{
				"obj": map[string]any{
					"foo": "bar",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			instanceCredentialsSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					tools.CredentialsSecretKey: credentialsBytes,
				},
			}

			Expect(adminClient.Create(ctx, instanceCredentialsSecret)).To(Succeed())

			instance = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceGUID,
					Namespace: testNamespace,
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					DisplayName: "mongodb-service-instance-name",
					Type:        "user-provided",
					Tags:        []string{},
				},
			}
			Expect(adminClient.Create(ctx, instance)).To(Succeed())
			Expect(k8s.Patch(ctx, adminClient, instance, func() {
				instance.Status.Credentials.Name = instanceCredentialsSecret.Name
			})).To(Succeed())
		})

		It("sets the ObservedGeneration status field", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.ObservedGeneration).To(Equal(binding.Generation))
			}).Should(Succeed())
		})

		It("sets the service-instance-type annotation to user-provided", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Annotations).To(HaveKeyWithValue(
					korifiv1alpha1.ServiceInstanceTypeAnnotation, "user-provided",
				))
			}).Should(Succeed())
		})

		It("does not set the plan-guid label", func() {
			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Labels).NotTo(HaveKey(
					korifiv1alpha1.PlanGUIDLabelKey,
				))
			}).Should(Succeed())
		})

		It("sets the env sercret to the instance credentials secret", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.EnvSecretRef.Name).To(Equal(instanceCredentialsSecret.Name))
			}).Should(Succeed())
		})

		It("creates the mount secret", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.MountSecretRef.Name).To(Equal(binding.Name))

				mountSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: binding.Namespace,
						Name:      binding.Status.MountSecretRef.Name,
					},
				}
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(mountSecret), mountSecret)).To(Succeed())
				g.Expect(mountSecret.Type).To(BeEquivalentTo("servicebinding.io/user-provided"))
				g.Expect(mountSecret.Data).To(MatchAllKeys(Keys{
					"type": BeEquivalentTo("user-provided"),
					"obj":  BeEquivalentTo(`{"foo":"bar"}`),
				}))
			}).Should(Succeed())
		})

		It("sets the binding Ready status condition to true", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionTrue)),
					HasReason(Equal("Ready")),
				)))
			}).Should(Succeed())
		})

		It("sets the mount secret ref in the binding status", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.MountSecretRef.Name).To(Equal(binding.Name))
			}).Should(Succeed())
		})

		When("the instance cretentials secret has a 'type' attribute", func() {
			BeforeEach(func() {
				credentialsBytes, err := json.Marshal(map[string]any{
					"type": "my-type",
				})
				Expect(err).NotTo(HaveOccurred())

				instanceCredentialsSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: testNamespace,
					},
					Data: map[string][]byte{
						tools.CredentialsSecretKey: credentialsBytes,
					},
				}

				Expect(adminClient.Create(ctx, instanceCredentialsSecret)).To(Succeed())

				Expect(k8s.Patch(ctx, adminClient, instance, func() {
					instance.Status.Credentials.Name = instanceCredentialsSecret.Name
				})).To(Succeed())
			})

			It("sets the mount secret type accordingly", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					mountSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: binding.Namespace,
							Name:      binding.Status.MountSecretRef.Name,
						},
					}
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(mountSecret), mountSecret)).To(Succeed())
					g.Expect(mountSecret.Type).To(BeEquivalentTo("servicebinding.io/my-type"))
					g.Expect(mountSecret.Data).To(MatchAllKeys(Keys{
						"type": BeEquivalentTo("my-type"),
					}))
				}).Should(Succeed())
			})
		})

		When("the env secret has its 'provider' attribute set", func() {
			BeforeEach(func() {
				credentialsBytes, err := json.Marshal(map[string]any{
					"provider": "my-provider",
				})
				Expect(err).NotTo(HaveOccurred())

				instanceCredentialsSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: testNamespace,
					},
					Data: map[string][]byte{
						tools.CredentialsSecretKey: credentialsBytes,
					},
				}

				Expect(adminClient.Create(ctx, instanceCredentialsSecret)).To(Succeed())

				Expect(k8s.Patch(ctx, adminClient, instance, func() {
					instance.Status.Credentials.Name = instanceCredentialsSecret.Name
				})).To(Succeed())
			})

			It("sets the specified provider in the mount secret", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					mountSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: binding.Namespace,
							Name:      binding.Status.MountSecretRef.Name,
						},
					}
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(mountSecret), mountSecret)).To(Succeed())
					g.Expect(mountSecret.Data).To(MatchKeys(IgnoreExtras, Keys{
						"provider": BeEquivalentTo("my-provider"),
					}))
				}).Should(Succeed())
			})
		})

		When("the service instance is not available", func() {
			BeforeEach(func() {
				Expect(adminClient.Delete(ctx, instance)).To(Succeed())
			})

			It("sets not ready status", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
					)))
				}).Should(Succeed())
			})
		})

		When("the service instance credentials secret is not available", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, adminClient, instance, func() {
					instance.Status.Credentials.Name = ""
				})).To(Succeed())
			})

			It("sets the Ready condition to false", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
					)))
				}).Should(Succeed())
			})
		})

		When("the binding is deleted", func() {
			JustBeforeEach(func() {
				Expect(adminClient.Delete(ctx, binding)).To(Succeed())
			})

			It("is deleted", func() {
				Eventually(func(g Gomega) {
					err := adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)
					g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})

	Describe("managed service bindings", func() {
		var (
			brokerClient *fake.BrokerClient

			serviceBroker   *korifiv1alpha1.CFServiceBroker
			serviceOffering *korifiv1alpha1.CFServiceOffering
			servicePlan     *korifiv1alpha1.CFServicePlan
			instance        *korifiv1alpha1.CFServiceInstance
		)

		BeforeEach(func() {
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

			serviceOffering = &korifiv1alpha1.CFServiceOffering{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: rootNamespace,
					Labels: map[string]string{
						korifiv1alpha1.RelServiceBrokerGUIDLabel: serviceBroker.Name,
					},
				},
				Spec: korifiv1alpha1.CFServiceOfferingSpec{
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
				},
			}
			Expect(adminClient.Create(ctx, servicePlan)).To(Succeed())

			brokerClient = new(fake.BrokerClient)
			brokerClientFactory.CreateClientReturns(brokerClient, nil)

			brokerClient.BindReturns(osbapi.BindResponse{
				Credentials: map[string]any{
					"foo": "bar",
				},
			}, nil)

			instance = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceGUID,
					Namespace: testNamespace,
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					DisplayName: "mongodb-service-instance-name",
					Type:        "managed",
					Tags:        []string{},
					PlanGUID:    servicePlan.Name,
				},
			}
			Expect(adminClient.Create(ctx, instance)).To(Succeed())
		})

		It("sets the ObservedGeneration status field", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.ObservedGeneration).To(Equal(binding.Generation))
			}).Should(Succeed())
		})

		It("sets the service-instance-type annotation to managed", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Annotations).To(HaveKeyWithValue(
					korifiv1alpha1.ServiceInstanceTypeAnnotation, "managed",
				))
			}).Should(Succeed())
		})

		It("sets the plan-guid label", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Labels).To(HaveKeyWithValue(
					korifiv1alpha1.PlanGUIDLabelKey, servicePlan.Name,
				))
			}).Should(Succeed())
		})

		It("binds the service", func() {
			Eventually(func(g Gomega) {
				g.Expect(brokerClient.BindCallCount()).To(BeNumerically(">", 0))
				_, payload := brokerClient.BindArgsForCall(0)
				g.Expect(payload).To(Equal(osbapi.BindPayload{
					InstanceID: instance.Name,
					BindingID:  binding.Name,
					BindRequest: osbapi.BindRequest{
						ServiceId: "service-offering-id",
						PlanID:    "service-plan-id",
						AppGUID:   cfAppGUID,
						BindResource: osbapi.BindResource{
							AppGUID: cfAppGUID,
						},
					},
				}))
			}).Should(Succeed())
		})

		When("the binding has parameters", func() {
			var paramsSecret *corev1.Secret

			BeforeEach(func() {
				paramsSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: binding.Namespace,
						Name:      uuid.NewString(),
					},
					Data: map[string][]byte{
						tools.ParametersSecretKey: []byte(`{"p1":"p1-value"}`),
					},
				}
				Expect(adminClient.Create(ctx, paramsSecret)).To(Succeed())

				Expect(k8s.Patch(ctx, adminClient, binding, func() {
					binding.Spec.Parameters.Name = paramsSecret.Name
				})).To(Succeed())
			})

			It("sends them to the broker on bind", func() {
				Eventually(func(g Gomega) {
					g.Expect(brokerClient.BindCallCount()).To(BeNumerically(">", 0))
					_, payload := brokerClient.BindArgsForCall(0)
					g.Expect(payload.Parameters).To(Equal(map[string]any{
						"p1": "p1-value",
					}))
				}).Should(Succeed())
			})

			When("the parameters secret does not exist", func() {
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, adminClient, binding, func() {
						binding.Spec.Parameters.Name = "not-valid"
					})).To(Succeed())
				})

				It("sets the ready condition to false", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
						g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
							HasType(Equal(korifiv1alpha1.StatusConditionReady)),
							HasStatus(Equal(metav1.ConditionFalse)),
							HasReason(Equal("InvalidParameters")),
						)))
					}).Should(Succeed())
				})
			})
		})

		It("does not check for binding last operation", func() {
			Consistently(func(g Gomega) {
				g.Expect(brokerClient.GetServiceBindingLastOperationCallCount()).To(BeZero())
			}).Should(Succeed())
		})

		It("does not get the binding", func() {
			Consistently(func(g Gomega) {
				g.Expect(brokerClient.GetServiceBindingCallCount()).To(BeZero())
			}).Should(Succeed())
		})

		It("creates the env secret", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.EnvSecretRef.Name).To(Equal(binding.Name))

				envSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: binding.Namespace,
						Name:      binding.Status.EnvSecretRef.Name,
					},
				}
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(envSecret), envSecret)).To(Succeed())
				g.Expect(envSecret.Type).To(BeEquivalentTo("Opaque"))
				g.Expect(envSecret.Data).To(MatchKeys(IgnoreExtras, Keys{
					tools.CredentialsSecretKey: BeEquivalentTo(`{"foo":"bar"}`),
				}))
				g.Expect(envSecret.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Name": Equal(binding.Name),
				})))
			}).Should(Succeed())
		})

		It("creates the mount secret", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.MountSecretRef.Name).NotTo(BeEmpty())

				mountSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: binding.Namespace,
						Name:      binding.Status.MountSecretRef.Name,
					},
				}
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(mountSecret), mountSecret)).To(Succeed())
				g.Expect(mountSecret.Type).To(BeEquivalentTo("servicebinding.io/managed"))
				g.Expect(mountSecret.Data).To(MatchKeys(IgnoreExtras, Keys{
					"foo": BeEquivalentTo("bar"),
				}))

				g.Expect(mountSecret.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Name": Equal(binding.Name),
				})))
			}).Should(Succeed())
		})

		When("binding is of type key", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, adminClient, binding, func() {
					binding.Spec.Type = korifiv1alpha1.CFServiceBindingTypeKey
				})).To(Succeed())
			})

			It("does not create a mount secret", func() {
				Consistently(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					g.Expect(binding.Status.MountSecretRef.Name).To(BeEmpty())

					secrets := &corev1.SecretList{}
					g.Expect(adminClient.List(ctx, secrets, client.InNamespace(binding.Namespace))).To(Succeed())

					g.Expect(secrets.Items).NotTo(ContainElement(
						MatchFields(IgnoreExtras, Fields{
							"Type": HavePrefix(credentials.ServiceBindingSecretTypePrefix),
						}),
					))
				}).Should(Succeed())
			})
		})

		When("the credentials contain type key", func() {
			BeforeEach(func() {
				brokerClient.BindReturns(osbapi.BindResponse{
					Credentials: map[string]any{
						"foo":  "bar",
						"type": "please-ignore-me",
					},
				}, nil)
			})

			It("creates the mount secret with type managed", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					g.Expect(binding.Status.MountSecretRef.Name).NotTo(BeEmpty())

					mountSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: binding.Namespace,
							Name:      binding.Status.MountSecretRef.Name,
						},
					}
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(mountSecret), mountSecret)).To(Succeed())
					g.Expect(mountSecret.Type).To(BeEquivalentTo("servicebinding.io/managed"))
				}).Should(Succeed())
			})
		})

		When("the binding credentials have been reconciled", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, adminClient, binding, func() {
					binding.Status.EnvSecretRef.Name = uuid.NewString()
					binding.Status.MountSecretRef.Name = uuid.NewString()
				})).To(Succeed())
			})

			It("does not request bind", func() {
				Consistently(func(g Gomega) {
					g.Expect(brokerClient.BindCallCount()).To(BeZero())
					g.Expect(brokerClient.GetServiceBindingLastOperationCallCount()).To(BeZero())
					g.Expect(brokerClient.GetServiceBindingCallCount()).To(BeZero())
				}).Should(Succeed())
			})
		})

		When("the binding has failed", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, adminClient, binding, func() {
					meta.SetStatusCondition(&binding.Status.Conditions, metav1.Condition{
						Type:   korifiv1alpha1.BindingFailedCondition,
						Status: metav1.ConditionTrue,
						Reason: "BindingFailed",
					})
				})).To(Succeed())
			})

			It("does not request bind", func() {
				Consistently(func(g Gomega) {
					g.Expect(brokerClient.BindCallCount()).To(BeZero())
					g.Expect(brokerClient.GetServiceBindingLastOperationCallCount()).To(BeZero())
					g.Expect(brokerClient.GetServiceBindingCallCount()).To(BeZero())
				}).Should(Succeed())
			})
		})

		When("binding is asynchronous", func() {
			BeforeEach(func() {
				brokerClient.BindReturns(osbapi.BindResponse{
					Operation: "operation-1",
					IsAsync:   true,
				}, nil)

				brokerClient.GetServiceBindingLastOperationReturns(osbapi.LastOperationResponse{
					State: "in-progress-or-whatever",
				}, nil)
			})

			It("sets the ready condition to false", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())

					g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasReason(Equal("BindingInProgress")),
					)))
				}).Should(Succeed())
			})

			It("keeps checking last operation", func() {
				Eventually(func(g Gomega) {
					g.Expect(brokerClient.GetServiceBindingLastOperationCallCount()).To(BeNumerically(">", 1))
					_, actualLastOpPayload := brokerClient.GetServiceBindingLastOperationArgsForCall(1)
					g.Expect(actualLastOpPayload).To(Equal(osbapi.GetBindingLastOperationRequest{
						InstanceID: instance.Name,
						BindingID:  binding.Name,
						GetLastOperationRequestParameters: osbapi.GetLastOperationRequestParameters{
							ServiceId: "service-offering-id",
							PlanID:    "service-plan-id",
							Operation: "operation-1",
						},
					}))
				}).Should(Succeed())
			})

			When("getting binding last operation fails", func() {
				BeforeEach(func() {
					brokerClient.GetServiceBindingLastOperationReturns(osbapi.LastOperationResponse{}, errors.New("get-last-op-failed"))
				})

				It("sets the ready condition to false", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())

						g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
							HasType(Equal(korifiv1alpha1.StatusConditionReady)),
							HasStatus(Equal(metav1.ConditionFalse)),
							HasReason(Equal("GetLastOperationFailed")),
							HasMessage(ContainSubstring("get-last-op-failed")),
						)))
					}).Should(Succeed())
				})
			})

			When("the last operation is failed", func() {
				BeforeEach(func() {
					brokerClient.GetServiceBindingLastOperationReturns(osbapi.LastOperationResponse{
						State:       "failed",
						Description: "last-operation-failed",
					}, nil)
				})

				It("fails the binding", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
						g.Expect(binding.Status.Conditions).To(ContainElements(
							SatisfyAll(
								HasType(Equal(korifiv1alpha1.StatusConditionReady)),
								HasStatus(Equal(metav1.ConditionFalse)),
							),
							SatisfyAll(
								HasType(Equal(korifiv1alpha1.BindingFailedCondition)),
								HasStatus(Equal(metav1.ConditionTrue)),
								HasReason(Equal("BindingFailed")),
								HasMessage(ContainSubstring("last-operation-failed")),
							),
						))
					}).Should(Succeed())
				})
			})

			When("last operation has succeeded", func() {
				BeforeEach(func() {
					brokerClient.GetServiceBindingLastOperationStub = func(context.Context, osbapi.GetBindingLastOperationRequest) (osbapi.LastOperationResponse, error) {
						brokerClient.BindReturns(osbapi.BindResponse{
							Credentials: map[string]any{
								"foo": "bar",
							},
						}, nil)

						return osbapi.LastOperationResponse{
							State: "succeeded",
						}, nil
					}
				})

				It("sets the ready condition to true", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())

						g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
							HasType(Equal(korifiv1alpha1.StatusConditionReady)),
							HasStatus(Equal(metav1.ConditionTrue)),
						)))
					}).Should(Succeed())
				})

				It("creates the env secret", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
						g.Expect(binding.Status.EnvSecretRef.Name).To(Equal(binding.Name))

						credentialsSecret := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: binding.Namespace,
								Name:      binding.Status.EnvSecretRef.Name,
							},
						}
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)).To(Succeed())
					}).Should(Succeed())
				})

				It("creates the mount secret", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
						g.Expect(binding.Status.MountSecretRef.Name).NotTo(BeEmpty())

						mountSecret := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: binding.Namespace,
								Name:      binding.Status.MountSecretRef.Name,
							},
						}
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(mountSecret), mountSecret)).To(Succeed())
					}).Should(Succeed())
				})
			})
		})

		Describe("bind failures", func() {
			When("bind fails with recoverable error", func() {
				BeforeEach(func() {
					brokerClient.BindReturns(osbapi.BindResponse{}, errors.New("binding-failed"))
				})

				It("keeps trying to bind", func() {
					Eventually(func(g Gomega) {
						g.Expect(brokerClient.BindCallCount()).To(BeNumerically(">", 1))
						_, payload := brokerClient.BindArgsForCall(1)
						g.Expect(payload).To(Equal(osbapi.BindPayload{
							InstanceID: instance.Name,
							BindingID:  binding.Name,
							BindRequest: osbapi.BindRequest{
								ServiceId: "service-offering-id",
								PlanID:    "service-plan-id",
								AppGUID:   cfAppGUID,
								BindResource: osbapi.BindResource{
									AppGUID: cfAppGUID,
								},
							},
						}))
					}).Should(Succeed())
				})

				It("sets ready to false", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
						g.Expect(binding.Status.Conditions).To(ContainElements(
							SatisfyAll(
								HasType(Equal(korifiv1alpha1.StatusConditionReady)),
								HasStatus(Equal(metav1.ConditionFalse)),
							),
						))
					}).Should(Succeed())
				})
			})

			When("bind fails with unrecoverable error", func() {
				BeforeEach(func() {
					brokerClient.BindReturns(osbapi.BindResponse{}, osbapi.UnrecoverableError{Status: http.StatusConflict})
				})

				It("fails the binding", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
						g.Expect(binding.Status.Conditions).To(ContainElements(
							SatisfyAll(
								HasType(Equal(korifiv1alpha1.StatusConditionReady)),
								HasStatus(Equal(metav1.ConditionFalse)),
							),
							SatisfyAll(
								HasType(Equal(korifiv1alpha1.BindingFailedCondition)),
								HasStatus(Equal(metav1.ConditionTrue)),
								HasReason(Equal("BindingFailed")),
							),
						))
					}).Should(Succeed())
				})
			})
		})

		Describe("binding deletion", func() {
			BeforeEach(func() {
				brokerClient.UnbindReturns(osbapi.UnbindResponse{}, nil)
			})

			JustBeforeEach(func() {
				Expect(k8sManager.GetClient().Delete(ctx, binding)).To(Succeed())
			})

			It("unbinds the binding with the broker", func() {
				Eventually(func(g Gomega) {
					g.Expect(brokerClient.UnbindCallCount()).NotTo(BeZero())
					_, actualUnbindRequest := brokerClient.UnbindArgsForCall(0)
					Expect(actualUnbindRequest).To(Equal(osbapi.UnbindPayload{
						BindingID:  binding.Name,
						InstanceID: instance.Name,
						UnbindRequestParameters: osbapi.UnbindRequestParameters{
							ServiceId: "service-offering-id",
							PlanID:    "service-plan-id",
						},
					}))
				}).Should(Succeed())
			})

			It("deletes the binding", func() {
				Eventually(func(g Gomega) {
					err := adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)
					g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				}).Should(Succeed())
			})

			When("noop deprovisioning is requested", func() {
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, adminClient, instance, func() {
						instance.Spec.NoopDeprovisioning = true
					})).To(Succeed())
				})

				It("does not contact the broker for unbinding", func() {
					Consistently(func(g Gomega) {
						g.Expect(brokerClient.UnbindCallCount()).To(Equal(0))
					}).Should(Succeed())
				})

				It("deletes the binding", func() {
					Eventually(func(g Gomega) {
						err := adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)
						g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
					}).Should(Succeed())
				})
			})

			When("unbind fails with recoverable error", func() {
				BeforeEach(func() {
					brokerClient.UnbindReturns(osbapi.UnbindResponse{}, errors.New("unbinding-failed"))
				})

				It("does not delete the binding", func() {
					Consistently(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					}).Should(Succeed())
				})

				It("keeps trying to unbind with the broker", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
						g.Expect(brokerClient.UnbindCallCount()).To(BeNumerically(">", 1))
					}).Should(Succeed())
				})
			})

			When("unbind fails with unrecoverable error", func() {
				BeforeEach(func() {
					brokerClient.UnbindReturns(osbapi.UnbindResponse{}, osbapi.UnrecoverableError{Status: http.StatusGone})
				})

				It("fails the binding", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())

						g.Expect(binding.Status.Conditions).To(ContainElements(
							SatisfyAll(
								HasType(Equal(korifiv1alpha1.StatusConditionReady)),
								HasStatus(Equal(metav1.ConditionFalse)),
							),
							SatisfyAll(
								HasType(Equal(korifiv1alpha1.UnbindingFailedCondition)),
								HasStatus(Equal(metav1.ConditionTrue)),
								HasReason(Equal("UnbindingFailed")),
								HasMessage(ContainSubstring("The server responded with status: 410")),
							),
						))
					}).Should(Succeed())
				})
			})

			When("the unbind is asynchronous", func() {
				BeforeEach(func() {
					brokerClient.UnbindReturns(osbapi.UnbindResponse{
						IsAsync:   true,
						Operation: "unbind-op",
					}, nil)
				})

				It("does not delete the binding", func() {
					Consistently(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					}).Should(Succeed())
				})

				When("the last operation is in progress", func() {
					BeforeEach(func() {
						brokerClient.GetServiceBindingLastOperationReturns(osbapi.LastOperationResponse{
							State: "in progress",
						}, nil)
					})

					It("sets the ready condition to false", func() {
						Eventually(func(g Gomega) {
							g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())

							g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
								HasType(Equal(korifiv1alpha1.StatusConditionReady)),
								HasStatus(Equal(metav1.ConditionFalse)),
								HasReason(Equal("UnbindingInProgress")),
							)))
						}).Should(Succeed())
					})

					It("keeps checking last operation", func() {
						Eventually(func(g Gomega) {
							g.Expect(brokerClient.GetServiceBindingLastOperationCallCount()).To(BeNumerically(">", 1))
							_, actualLastOpPayload := brokerClient.GetServiceBindingLastOperationArgsForCall(1)
							g.Expect(actualLastOpPayload).To(Equal(osbapi.GetBindingLastOperationRequest{
								InstanceID: instance.Name,
								BindingID:  binding.Name,
								GetLastOperationRequestParameters: osbapi.GetLastOperationRequestParameters{
									ServiceId: "service-offering-id",
									PlanID:    "service-plan-id",
									Operation: "unbind-op",
								},
							}))
						}).Should(Succeed())
					})
				})

				When("the last operation is failed", func() {
					BeforeEach(func() {
						brokerClient.GetServiceBindingLastOperationReturns(osbapi.LastOperationResponse{
							State: "failed",
						}, nil)
					})

					It("sets the failed condition", func() {
						Eventually(func(g Gomega) {
							g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())

							g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
								HasType(Equal(korifiv1alpha1.StatusConditionReady)),
								HasStatus(Equal(metav1.ConditionFalse)),
							)))

							g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
								HasType(Equal(korifiv1alpha1.UnbindingFailedCondition)),
								HasStatus(Equal(metav1.ConditionTrue)),
							)))
						}).Should(Succeed())
					})
				})

				When("the broker unbind succeeds", func() {
					BeforeEach(func() {
						brokerClient.UnbindReturns(osbapi.UnbindResponse{}, nil)
					})

					It("deletes the binding", func() {
						Eventually(func(g Gomega) {
							err := adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)
							g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
						}).Should(Succeed())
					})
				})

				When("the binding is gone", func() {
					BeforeEach(func() {
						brokerClient.UnbindReturns(osbapi.UnbindResponse{}, osbapi.GoneError{})
					})

					It("deletes the binding", func() {
						Eventually(func(g Gomega) {
							err := adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)
							g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
						}).Should(Succeed())
					})
				})
			})
		})
	})
})
