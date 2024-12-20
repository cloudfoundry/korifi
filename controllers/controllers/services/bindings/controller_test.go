package bindings_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi/fake"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "code.cloudfoundry.org/korifi/tests/matchers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	servicebindingv1beta1 "github.com/servicebinding/runtime/apis/v1beta1"
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

		It("sets an owner reference from the instance to the binding", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Name": Equal(instance.Name),
				})))
			}).Should(Succeed())
		})

		It("sets the service-instance-type annotation to user-provided", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Annotations).To(HaveKeyWithValue(
					korifiv1alpha1.ServiceInstanceTypeAnnotationKey, "user-provided",
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

		It("sets the binding status credentials name to the instance credentials secret", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.Credentials.Name).To(Equal(instanceCredentialsSecret.Name))
			}).Should(Succeed())
		})

		It("creates the binding secret", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.Binding.Name).To(Equal(binding.Name))

				bindingSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: binding.Namespace,
						Name:      binding.Status.Binding.Name,
					},
				}
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(bindingSecret), bindingSecret)).To(Succeed())
				g.Expect(bindingSecret.Type).To(BeEquivalentTo("servicebinding.io/user-provided"))
				g.Expect(bindingSecret.Data).To(MatchAllKeys(Keys{
					"type": BeEquivalentTo("user-provided"),
					"obj":  BeEquivalentTo(`{"foo":"bar"}`),
				}))
			}).Should(Succeed())
		})

		It("sets the binding Ready status condition to false", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
					HasReason(Equal("ServiceBindingNotReady")),
				)))
			}).Should(Succeed())
		})

		It("creates a servicebinding.io ServiceBinding", func() {
			Eventually(func(g Gomega) {
				sbServiceBinding := &servicebindingv1beta1.ServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      fmt.Sprintf("cf-binding-%s", binding.Name),
					},
				}
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())

				g.Expect(sbServiceBinding.Spec.Name).To(Equal(binding.Name))
				g.Expect(sbServiceBinding.Spec.Type).To(Equal("user-provided"))
				g.Expect(sbServiceBinding.Spec.Provider).To(BeEmpty())

				g.Expect(sbServiceBinding.Labels).To(SatisfyAll(
					HaveKeyWithValue(korifiv1alpha1.ServiceBindingGUIDLabel, binding.Name),
					HaveKeyWithValue(korifiv1alpha1.CFAppGUIDLabelKey, cfAppGUID),
					HaveKeyWithValue(korifiv1alpha1.ServiceCredentialBindingTypeLabel, "app"),
				))

				g.Expect(sbServiceBinding.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Kind": Equal("CFServiceBinding"),
					"Name": Equal(binding.Name),
				})))

				g.Expect(sbServiceBinding.Spec.Workload).To(MatchFields(IgnoreExtras, Fields{
					"APIVersion": Equal("apps/v1"),
					"Kind":       Equal("StatefulSet"),
					"Selector": PointTo(Equal(metav1.LabelSelector{
						MatchLabels: map[string]string{
							korifiv1alpha1.CFAppGUIDLabelKey: cfAppGUID,
						},
					})),
				}))

				g.Expect(sbServiceBinding.Spec.Service).To(MatchFields(IgnoreExtras, Fields{
					"APIVersion": Equal("korifi.cloudfoundry.org/v1alpha1"),
					"Kind":       Equal("CFServiceBinding"),
					"Name":       Equal(binding.Name),
				}))
			}).Should(Succeed())
		})

		It("sets the binding status binding name to the binding secret name", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.Binding.Name).To(Equal(binding.Name))
			}).Should(Succeed())
		})

		When("the CFServiceBinding has a displayName set", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, binding, func() {
					binding.Spec.DisplayName = tools.PtrTo("a-custom-binding-name")
				})).To(Succeed())
			})

			It("sets the displayName as the name on the servicebinding.io ServiceBinding", func() {
				Eventually(func(g Gomega) {
					sbServiceBinding := &servicebindingv1beta1.ServiceBinding{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNamespace,
							Name:      fmt.Sprintf("cf-binding-%s", binding.Name),
						},
					}
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())
					g.Expect(sbServiceBinding.Spec.Name).To(Equal("a-custom-binding-name"))
				}).Should(Succeed())
			})
		})

		When("the servicebinding.io binding is ready", func() {
			var sbBinding *servicebindingv1beta1.ServiceBinding

			BeforeEach(func() {
				sbBinding = &servicebindingv1beta1.ServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      fmt.Sprintf("cf-binding-%s", binding.Name),
					},
				}
				Expect(adminClient.Create(ctx, sbBinding)).To(Succeed())

				Expect(k8s.Patch(ctx, adminClient, sbBinding, func() {
					meta.SetStatusCondition(&sbBinding.Status.Conditions, metav1.Condition{
						Type:    "Ready",
						Status:  metav1.ConditionTrue,
						Reason:  "whatever",
						Message: "",
					})

					// Patching the object increments its generation. In order to
					// ensure that the observed generation matches the generation,
					// we set the observed generation to `generation + 1`
					sbBinding.Status.ObservedGeneration = sbBinding.Generation + 1
				})).To(Succeed())
			})

			It("sets the binding Ready status condition to true", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionTrue)),
					)))
				}).Should(Succeed())
			})

			When("the servicebinding.io binding ready status is outdated", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, adminClient, sbBinding, func() {
						sbBinding.Status.ObservedGeneration = sbBinding.Generation - 1
					})).To(Succeed())
				})

				It("sets the binding Ready status condition to false", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
						g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
							HasType(Equal(korifiv1alpha1.StatusConditionReady)),
							HasStatus(Equal(metav1.ConditionFalse)),
							HasReason(Equal("ServiceBindingNotReady")),
						)))
					}).Should(Succeed())
					Consistently(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
						g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
							HasType(Equal(korifiv1alpha1.StatusConditionReady)),
							HasStatus(Equal(metav1.ConditionFalse)),
							HasReason(Equal("ServiceBindingNotReady")),
						)))
					}).Should(Succeed())
				})
			})
		})

		When("the credentials secret has its 'type' attribute set", func() {
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

			It("sets the specified type as servicebinding.io secret type", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					bindingSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: binding.Namespace,
							Name:      binding.Status.Binding.Name,
						},
					}
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(bindingSecret), bindingSecret)).To(Succeed())
					g.Expect(bindingSecret.Type).To(BeEquivalentTo("servicebinding.io/my-type"))
					g.Expect(bindingSecret.Data).To(MatchAllKeys(Keys{
						"type": BeEquivalentTo("my-type"),
					}))
				}).Should(Succeed())
			})

			It("sets the provided type into the servicebinding.io object", func() {
				Eventually(func(g Gomega) {
					sbServiceBinding := &servicebindingv1beta1.ServiceBinding{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNamespace,
							Name:      fmt.Sprintf("cf-binding-%s", binding.Name),
						},
					}
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())
					g.Expect(sbServiceBinding.Spec.Type).To(Equal("my-type"))
				}).Should(Succeed())
			})
		})

		When("the credentials secret has its 'provider' attribute set", func() {
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

			It("sets the specified provider in the binding secret", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					bindingSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: binding.Namespace,
							Name:      binding.Status.Binding.Name,
						},
					}
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(bindingSecret), bindingSecret)).To(Succeed())
					g.Expect(bindingSecret.Data).To(MatchKeys(IgnoreExtras, Keys{
						"provider": BeEquivalentTo("my-provider"),
					}))
				}).Should(Succeed())
			})

			It("sets the provided provider into the servicebinding.io object", func() {
				Eventually(func(g Gomega) {
					sbServiceBinding := &servicebindingv1beta1.ServiceBinding{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNamespace,
							Name:      fmt.Sprintf("cf-binding-%s", binding.Name),
						},
					}
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())
					g.Expect(sbServiceBinding.Spec.Provider).To(Equal("my-provider"))
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

		When("the credentials secret is not available", func() {
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
					ServiceBroker: services.ServiceBroker{
						Name: "my-service-broker",
					},
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

		It("sets an owner reference from the instance to the binding", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Name": Equal(instance.Name),
				})))
			}).Should(Succeed())
		})

		It("sets the service-instance-type annotation to managed", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Annotations).To(HaveKeyWithValue(
					korifiv1alpha1.ServiceInstanceTypeAnnotationKey, "managed",
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

		It("creates the credentials secret", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.Credentials.Name).To(Equal(binding.Name))

				credentialsSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: binding.Namespace,
						Name:      binding.Status.Credentials.Name,
					},
				}
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)).To(Succeed())
				g.Expect(credentialsSecret.Type).To(BeEquivalentTo("Opaque"))
				g.Expect(credentialsSecret.Data).To(MatchKeys(IgnoreExtras, Keys{
					tools.CredentialsSecretKey: BeEquivalentTo(`{"foo":"bar"}`),
				}))
				g.Expect(credentialsSecret.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Name": Equal(binding.Name),
				})))
			}).Should(Succeed())
		})

		It("creates a servicebinding.io ServiceBinding", func() {
			Eventually(func(g Gomega) {
				sbServiceBinding := &servicebindingv1beta1.ServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      fmt.Sprintf("cf-binding-%s", binding.Name),
					},
				}
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())

				g.Expect(sbServiceBinding.Spec.Name).To(Equal(binding.Status.Binding.Name))
				g.Expect(sbServiceBinding.Spec.Type).To(Equal("managed"))
			}).Should(Succeed())
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

			It("sets the servicebinding.io type to managed (ignoring the type credentials key)", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					sbServiceBinding := &servicebindingv1beta1.ServiceBinding{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNamespace,
							Name:      fmt.Sprintf("cf-binding-%s", binding.Name),
						},
					}
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())

					g.Expect(sbServiceBinding.Spec.Type).To(Equal("managed"))
				}).Should(Succeed())
			})
		})

		When("the credentials contain provider key", func() {
			BeforeEach(func() {
				brokerClient.BindReturns(osbapi.BindResponse{
					Credentials: map[string]any{
						"foo":      "bar",
						"provider": "please-ignore-me",
					},
				}, nil)
			})

			It("does not set the servicebinding.io provider type (ignoring the provider credentials key)", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					sbServiceBinding := &servicebindingv1beta1.ServiceBinding{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNamespace,
							Name:      fmt.Sprintf("cf-binding-%s", binding.Name),
						},
					}
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())

					g.Expect(sbServiceBinding.Spec.Provider).To(BeEmpty())
				}).Should(Succeed())
			})
		})

		It("creates the servicebinding.io secret", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.Binding.Name).NotTo(BeEmpty())

				bindingSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: binding.Namespace,
						Name:      binding.Status.Binding.Name,
					},
				}
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(bindingSecret), bindingSecret)).To(Succeed())
				g.Expect(bindingSecret.Type).To(BeEquivalentTo("servicebinding.io/managed"))
				g.Expect(bindingSecret.Data).To(MatchKeys(IgnoreExtras, Keys{
					"foo": BeEquivalentTo("bar"),
				}))

				g.Expect(bindingSecret.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Name": Equal(binding.Name),
				})))
			}).Should(Succeed())
		})

		When("the binding credentials have been reconciled", func() {
			BeforeEach(func() {
				Expect(k8s.Patch(ctx, adminClient, binding, func() {
					binding.Status.Credentials.Name = uuid.NewString()
					binding.Status.Binding.Name = uuid.NewString()
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
					State: "in progress",
				}, nil)
			})

			It("sets the ready condition to false", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())

					g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
					)))
				}).Should(Succeed())
			})

			It("sets the BindRequested condition", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.BindingRequestedCondition)),
						HasStatus(Equal(metav1.ConditionTrue)),
					)))
				}).Should(Succeed())

				Consistently(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.BindingRequestedCondition)),
						HasStatus(Equal(metav1.ConditionTrue)),
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
					brokerClient.GetServiceBindingLastOperationReturns(osbapi.LastOperationResponse{
						State: "succeeded",
					}, nil)

					brokerClient.GetServiceBindingReturns(osbapi.GetBindingResponse{
						Credentials: map[string]any{
							"foo": "bar",
						},
					}, nil)
				})

				It("creates the credentials secret", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
						g.Expect(binding.Status.Credentials.Name).To(Equal(binding.Name))

						credentialsSecret := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: binding.Namespace,
								Name:      binding.Status.Credentials.Name,
							},
						}
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)).To(Succeed())
						g.Expect(credentialsSecret.Type).To(BeEquivalentTo("Opaque"))
						g.Expect(credentialsSecret.Data).To(MatchKeys(IgnoreExtras, Keys{
							tools.CredentialsSecretKey: BeEquivalentTo(`{"foo":"bar"}`),
						}))
						g.Expect(credentialsSecret.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
							"Name": Equal(binding.Name),
						})))
					}).Should(Succeed())
				})

				It("creates the servicebinding.io secret", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
						g.Expect(binding.Status.Binding.Name).NotTo(BeEmpty())

						bindingSecret := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: binding.Namespace,
								Name:      binding.Status.Binding.Name,
							},
						}
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(bindingSecret), bindingSecret)).To(Succeed())
						g.Expect(bindingSecret.Type).To(BeEquivalentTo("servicebinding.io/managed"))
						g.Expect(bindingSecret.Data).To(MatchKeys(IgnoreExtras, Keys{
							"foo": BeEquivalentTo("bar"),
						}))

						g.Expect(bindingSecret.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
							"Name": Equal(binding.Name),
						})))
					}).Should(Succeed())
				})

				When("getting the binding fails", func() {
					BeforeEach(func() {
						brokerClient.GetServiceBindingReturns(osbapi.GetBindingResponse{}, errors.New("get-binding-err"))
					})

					It("sets the ready condition to false", func() {
						Eventually(func(g Gomega) {
							g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())

							g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
								HasType(Equal(korifiv1alpha1.StatusConditionReady)),
								HasStatus(Equal(metav1.ConditionFalse)),
								HasMessage(ContainSubstring("get-binding-err")),
							)))
						}).Should(Succeed())
					})
				})
			})
		})

		When("binding fails with the broker", func() {
			BeforeEach(func() {
				brokerClient.BindReturns(osbapi.BindResponse{}, errors.New("binding-failed"))
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
							HasMessage(ContainSubstring("binding-failed")),
						),
					))
				}).Should(Succeed())
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
					err := adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)
					g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())

					g.Expect(brokerClient.UnbindCallCount()).To(Equal(1))
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

			Describe("unbind failure", func() {
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
			})

			When("the unbind is asynchronous", func() {
				BeforeEach(func() {
					brokerClient.UnbindReturns(osbapi.UnbindResponse{
						IsAsync:   true,
						Operation: "unbind-op",
					}, nil)
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
			})
		})
	})
})
