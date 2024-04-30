package bindings_test

import (
	"encoding/json"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/bindings"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "code.cloudfoundry.org/korifi/tests/matchers"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	servicebindingv1beta1 "github.com/servicebinding/runtime/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFServiceBinding", func() {
	var (
		testNamespace             string
		cfApp                     *korifiv1alpha1.CFApp
		instance                  *korifiv1alpha1.CFServiceInstance
		binding                   *korifiv1alpha1.CFServiceBinding
		instanceCredentialsSecret *corev1.Secret
	)

	BeforeEach(func() {
		testNamespace = uuid.NewString()
		Expect(adminClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		})).To(Succeed())

		cfApp = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  uuid.NewString(),
				DesiredState: korifiv1alpha1.StartedState,
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "docker",
				},
			},
		}
		Expect(
			adminClient.Create(ctx, cfApp),
		).To(Succeed())

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
				korifiv1alpha1.CredentialsSecretKey: credentialsBytes,
			},
		}

		Expect(adminClient.Create(ctx, instanceCredentialsSecret)).To(Succeed())

		instance = &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
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

		binding = &korifiv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFServiceBindingSpec{
				Service: corev1.ObjectReference{
					Kind:       "ServiceInstance",
					Name:       instance.Name,
					APIVersion: "korifi.cloudfoundry.org/v1alpha1",
				},
				AppRef: corev1.LocalObjectReference{
					Name: cfApp.Name,
				},
			},
		}
		Expect(adminClient.Create(ctx, binding)).To(Succeed())
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

			g.Expect(sbServiceBinding.Spec.Name).To(Equal(binding.Name))
			g.Expect(sbServiceBinding.Spec.Type).To(Equal("user-provided"))
			g.Expect(sbServiceBinding.Spec.Provider).To(BeEmpty())

			g.Expect(sbServiceBinding.Labels).To(SatisfyAll(
				HaveKeyWithValue(bindings.ServiceBindingGUIDLabel, binding.Name),
				HaveKeyWithValue(korifiv1alpha1.CFAppGUIDLabelKey, cfApp.Name),
				HaveKeyWithValue(bindings.ServiceCredentialBindingTypeLabel, "app"),
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
						korifiv1alpha1.CFAppGUIDLabelKey: cfApp.Name,
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
		BeforeEach(func() {
			Eventually(func(g Gomega) {
				sbServiceBinding := &servicebindingv1beta1.ServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      fmt.Sprintf("cf-binding-%s", binding.Name),
					},
				}
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())
				g.Expect(k8s.Patch(ctx, adminClient, sbServiceBinding, func() {
					meta.SetStatusCondition(&sbServiceBinding.Status.Conditions, metav1.Condition{
						Type:    "Ready",
						Status:  metav1.ConditionTrue,
						Reason:  "whatever",
						Message: "",
					})
					sbServiceBinding.Status.ObservedGeneration = sbServiceBinding.Generation
				})).To(Succeed())
			}).Should(Succeed())
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
				Eventually(func(g Gomega) {
					sbServiceBinding := &servicebindingv1beta1.ServiceBinding{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: testNamespace,
							Name:      fmt.Sprintf("cf-binding-%s", binding.Name),
						},
					}
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())
					g.Expect(k8s.Patch(ctx, adminClient, sbServiceBinding, func() {
						sbServiceBinding.Status.ObservedGeneration = sbServiceBinding.Generation - 1
					})).To(Succeed())
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

	When("the servicebinding.io binding is not ready", func() {
		BeforeEach(func() {
			Eventually(func(g Gomega) {
				sbServiceBinding := &servicebindingv1beta1.ServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      fmt.Sprintf("cf-binding-%s", binding.Name),
					},
				}
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())
				g.Expect(k8s.Patch(ctx, adminClient, sbServiceBinding, func() {
					meta.SetStatusCondition(&sbServiceBinding.Status.Conditions, metav1.Condition{
						Type:    "Ready",
						Status:  metav1.ConditionFalse,
						Reason:  "whatever",
						Message: "",
					})
				})).To(Succeed())
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
	})

	When("the credentials secret has its 'type' attribute set", func() {
		BeforeEach(func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				bindingSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: binding.Namespace,
						Name:      binding.Status.Binding.Name,
					},
				}
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(bindingSecret), bindingSecret)).To(Succeed())
				g.Expect(bindingSecret.Type).To(BeEquivalentTo("servicebinding.io/user-provided"))

				sbServiceBinding := &servicebindingv1beta1.ServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      fmt.Sprintf("cf-binding-%s", binding.Name),
					},
				}
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(sbServiceBinding), sbServiceBinding)).To(Succeed())
			}).Should(Succeed())

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
					korifiv1alpha1.CredentialsSecretKey: credentialsBytes,
				},
			}

			Expect(adminClient.Create(ctx, instanceCredentialsSecret)).To(Succeed())

			Expect(k8s.Patch(ctx, adminClient, instance, func() {
				instance.Status.Credentials.Name = instanceCredentialsSecret.Name
			})).To(Succeed())
		})

		It("sets the specified type as secret type", func() {
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
				fmt.Fprintf(GinkgoWriter, "servicebinding.io: %+v\n", sbServiceBinding)
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
					korifiv1alpha1.CredentialsSecretKey: credentialsBytes,
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

	When("the CFApp is not available", func() {
		BeforeEach(func() {
			Expect(adminClient.Delete(ctx, cfApp)).To(Succeed())
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

	When("the binding references a 'legacy' instance credentials secret", func() {
		JustBeforeEach(func() {
			Expect(k8s.Patch(ctx, adminClient, instance, func() {
				instance.Spec.SecretName = instance.Name
				instance.Status.Credentials.Name = instance.Name
			})).To(Succeed())

			Eventually(func(g Gomega) {
				g.Expect(k8s.Patch(ctx, adminClient, binding, func() {
					binding.Status.Binding.Name = instance.Name
				})).To(Succeed())

				// Ensure that the binding controller has observed the patch operation above
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Generation).To(Equal(binding.Status.ObservedGeneration))
				g.Expect(binding.Status.Binding.Name).To(Equal(instance.Name))
			}).Should(Succeed())
		})

		It("sets the binding Ready status condition to false", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
				g.Expect(binding.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
				)))
			}).Should(Succeed())
		})

		When("the referenced legacy binding secret exists", func() {
			BeforeEach(func() {
				Expect(adminClient.Create(ctx, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      instance.Name,
						Namespace: testNamespace,
					},
				})).To(Succeed())
			})

			It("does not update the binding status", func() {
				Consistently(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(binding), binding)).To(Succeed())
					g.Expect(binding.Status.Binding.Name).To(Equal(instance.Name))
				}).Should(Succeed())
			})
		})
	})
})
