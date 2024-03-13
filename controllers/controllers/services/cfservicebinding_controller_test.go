package services_test

import (
	"encoding/json"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	servicebindingv1beta1 "github.com/servicebinding/runtime/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFServiceBinding", func() {
	var (
		namespace            *corev1.Namespace
		cfAppGUID            string
		desiredCFApp         *korifiv1alpha1.CFApp
		cfServiceInstance    *korifiv1alpha1.CFServiceInstance
		credentialsSecret    *corev1.Secret
		cfServiceBinding     *korifiv1alpha1.CFServiceBinding
		cfServiceBindingGUID string
		credentialsData      map[string]any
	)

	BeforeEach(func() {
		namespace = BuildNamespaceObject(GenerateGUID())
		Expect(
			adminClient.Create(ctx, namespace),
		).To(Succeed())

		cfAppGUID = GenerateGUID()
		desiredCFApp = BuildCFAppCRObject(cfAppGUID, namespace.Name)
		Expect(
			adminClient.Create(ctx, desiredCFApp),
		).To(Succeed())

		Expect(k8s.Patch(ctx, adminClient, desiredCFApp, func() {
			desiredCFApp.Status = korifiv1alpha1.CFAppStatus{
				Conditions:                nil,
				VCAPServicesSecretName:    "foo",
				VCAPApplicationSecretName: "bar",
			}
			meta.SetStatusCondition(&desiredCFApp.Status.Conditions, metav1.Condition{
				Type:   "Ready",
				Status: metav1.ConditionTrue,
				Reason: "testing",
			})
		})).To(Succeed())

		credentialsData = map[string]any{
			"type":     "my-type",
			"provider": "my-provider",
			"obj": map[string]any{
				"foo": "bar",
			},
		}
		credentialsSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: namespace.Name,
			},
		}

		cfServiceInstance = &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: namespace.Name,
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "mongodb-service-instance-name",
				SecretName:  credentialsSecret.Name,
				Type:        "user-provided",
				Tags:        []string{},
			},
		}

		cfServiceBindingGUID = GenerateGUID()
		cfServiceBinding = &korifiv1alpha1.CFServiceBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfServiceBindingGUID,
				Namespace: namespace.Name,
			},
			Spec: korifiv1alpha1.CFServiceBindingSpec{
				Service: corev1.ObjectReference{
					Kind:       "ServiceInstance",
					Name:       cfServiceInstance.Name,
					APIVersion: "korifi.cloudfoundry.org/v1alpha1",
				},
				AppRef: corev1.LocalObjectReference{
					Name: cfAppGUID,
				},
			},
		}
	})

	JustBeforeEach(func() {
		Expect(adminClient.Create(ctx, cfServiceInstance)).To(Succeed())
		Expect(adminClient.Create(ctx, cfServiceBinding)).To(Succeed())
	})

	It("sets binding secret not available condition", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBinding), cfServiceBinding)).To(Succeed())
			g.Expect(cfServiceBinding.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Type":               Equal(services.BindingSecretAvailableCondition),
				"Status":             Equal(metav1.ConditionFalse),
				"Reason":             Equal("CredentialsSecretNotAvailable"),
				"ObservedGeneration": Equal(cfServiceBinding.Generation),
			})))
		}).Should(Succeed())
	})

	When("the credentials secret is available", func() {
		BeforeEach(func() {
			credentialsBytes, err := json.Marshal(credentialsData)
			Expect(err).NotTo(HaveOccurred())
			credentialsSecret.Data = map[string][]byte{
				korifiv1alpha1.CredentialsSecretKey: credentialsBytes,
			}
		})

		JustBeforeEach(func() {
			Expect(adminClient.Create(ctx, credentialsSecret)).To(Succeed())
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(cfServiceInstance.Status.Conditions, services.CredentialsSecretAvailableCondition)).To(BeTrue())
			}).Should(Succeed())
		})

		It("sets an owner reference from the service instance to the service binding", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBinding), cfServiceBinding)).To(Succeed())
				fmt.Fprintf(GinkgoWriter, "cfServiceInstance = %+v\n", cfServiceInstance)

				g.Expect(cfServiceBinding.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Name": Equal(cfServiceInstance.Name),
				})))
			}).Should(Succeed())
		})

		It("reconciles the service instance credentials secret", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBinding), cfServiceBinding)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(cfServiceBinding.Status.Conditions, services.BindingSecretAvailableCondition)).To(BeTrue())

				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())
				g.Expect(cfServiceInstance.Status.Credentials.Name).NotTo(BeEmpty())
			}).Should(Succeed())

			Expect(cfServiceBinding.Status.Credentials.Name).To(Equal(cfServiceInstance.Status.Credentials.Name))

			Expect(cfServiceBinding.Status.Binding.Name).NotTo(BeEmpty())

			bindingSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cfServiceBinding.Namespace,
					Name:      cfServiceBinding.Status.Binding.Name,
				},
			}
			Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(bindingSecret), bindingSecret)).To(Succeed())
			Expect(bindingSecret.Type).To(BeEquivalentTo(services.ServiceBindingSecretTypePrefix + "my-type"))
			Expect(bindingSecret.Data).To(MatchAllKeys(Keys{
				"type":     Equal([]byte("my-type")),
				"provider": Equal([]byte("my-provider")),
				"obj":      Equal([]byte(`{"foo":"bar"}`)),
			}))
		})

		It("creates a servicebinding.io ServiceBinding", func() {
			Eventually(func(g Gomega) {
				sbServiceBinding := servicebindingv1beta1.ServiceBinding{}
				g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("cf-binding-%s", cfServiceBindingGUID), Namespace: namespace.Name}, &sbServiceBinding)).To(Succeed())
				g.Expect(sbServiceBinding).To(MatchFields(IgnoreExtras, Fields{
					"ObjectMeta": MatchFields(IgnoreExtras, Fields{
						"Name":      Equal(fmt.Sprintf("cf-binding-%s", cfServiceBindingGUID)),
						"Namespace": Equal(namespace.Name),
						"Labels": MatchKeys(IgnoreExtras, Keys{
							services.ServiceBindingGUIDLabel:           Equal(cfServiceBindingGUID),
							korifiv1alpha1.CFAppGUIDLabelKey:           Equal(cfAppGUID),
							services.ServiceCredentialBindingTypeLabel: Equal("app"),
						}),
						"OwnerReferences": ContainElement(MatchFields(IgnoreExtras, Fields{
							"APIVersion": Equal("korifi.cloudfoundry.org/v1alpha1"),
							"Kind":       Equal("CFServiceBinding"),
							"Name":       Equal(cfServiceBindingGUID),
						})),
					}),
					"Spec": MatchFields(IgnoreExtras, Fields{
						"Name":     Equal(cfServiceBinding.Name),
						"Type":     Equal("my-type"),
						"Provider": Equal("my-provider"),
						"Workload": MatchFields(IgnoreExtras, Fields{
							"APIVersion": Equal("apps/v1"),
							"Kind":       Equal("StatefulSet"),
							"Selector": PointTo(MatchFields(IgnoreExtras, Fields{
								"MatchLabels": MatchKeys(IgnoreExtras, Keys{
									korifiv1alpha1.CFAppGUIDLabelKey: Equal(cfAppGUID),
								}),
							})),
						}),
						"Service": MatchFields(IgnoreExtras, Fields{
							"APIVersion": Equal("korifi.cloudfoundry.org/v1alpha1"),
							"Kind":       Equal("CFServiceBinding"),
							"Name":       Equal(cfServiceBindingGUID),
						}),
					}),
				}))
			}).Should(Succeed())
		})

		It("sets the ObservedGeneration status field", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBinding), cfServiceBinding)).To(Succeed())
				g.Expect(cfServiceBinding.Status.ObservedGeneration).To(Equal(cfServiceBinding.Generation))
			}).Should(Succeed())
		})

		When("the CFServiceBinding has a displayName set", func() {
			var bindingName string

			BeforeEach(func() {
				cfServiceBindingGUID = GenerateGUID()
				bindingName = "a-custom-binding-name"
				cfServiceBinding = &korifiv1alpha1.CFServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfServiceBindingGUID,
						Namespace: namespace.Name,
					},
					Spec: korifiv1alpha1.CFServiceBindingSpec{
						DisplayName: &bindingName,
						Service: corev1.ObjectReference{
							Kind:       "ServiceInstance",
							Name:       cfServiceInstance.Name,
							APIVersion: "korifi.cloudfoundry.org/v1alpha1",
						},
						AppRef: corev1.LocalObjectReference{
							Name: cfAppGUID,
						},
					},
				}
			})

			It("sets the displayName as the name on the servicebinding.io ServiceBinding", func() {
				Eventually(func(g Gomega) {
					sbServiceBinding := servicebindingv1beta1.ServiceBinding{}
					g.Expect(adminClient.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("cf-binding-%s", cfServiceBindingGUID), Namespace: namespace.Name}, &sbServiceBinding)).To(Succeed())
					g.Expect(sbServiceBinding).To(MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"Name": Equal(bindingName),
						}),
					}))
				}).Should(Succeed())
			})
		})

		When("the credentials secret does not exist", func() {
			BeforeEach(func() {
				cfServiceBinding.Spec.Service.Name = "does-not-exist"
			})

			It("does not set binding name in the service binding status", func() {
				Consistently(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBinding), cfServiceBinding)).To(Succeed())
					g.Expect(cfServiceBinding.Status.Binding.Name).To(BeEmpty())
				}).Should(Succeed())
			})
		})

		When("the credentials change", func() {
			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBinding), cfServiceBinding)).To(Succeed())
					g.Expect(cfServiceBinding.Status.Binding.Name).To(Equal(cfServiceBinding.Name))
				}).Should(Succeed())
				Expect(k8s.Patch(ctx, adminClient, credentialsSecret, func() {
					credentialsSecret.Data = map[string][]byte{
						korifiv1alpha1.CredentialsSecretKey: []byte(`{"type":"my-type","provider": "your-provider"}`),
					}
				})).To(Succeed())
			})

			It("updates the binding secret", func() {
				Eventually(func(g Gomega) {
					bindingSecret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: cfServiceBinding.Namespace,
							Name:      cfServiceBinding.Name,
						},
					}
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(bindingSecret), bindingSecret)).To(Succeed())
					g.Expect(bindingSecret.Data).To(MatchAllKeys(Keys{
						"type":     Equal([]byte("my-type")),
						"provider": Equal([]byte("your-provider")),
					}))
				}).Should(Succeed())
			})
		})

		When("the service binding references the legacy service instance creadentials secret", func() {
			BeforeEach(func() {
				credentialsSecret.Name = cfServiceInstance.Name
				cfServiceInstance.Spec.SecretName = cfServiceInstance.Name
			})

			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBinding), cfServiceBinding)).To(Succeed())
					g.Expect(cfServiceBinding.Status.Binding.Name).NotTo(BeEmpty())
				}).Should(Succeed())
				Expect(k8s.Patch(ctx, adminClient, cfServiceBinding, func() {
					cfServiceBinding.Status.Binding.Name = cfServiceInstance.Name
				})).To(Succeed())
			})

			It("does not create a new binding secret", func() {
				Consistently(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBinding), cfServiceBinding)).To(Succeed())
					g.Expect(cfServiceBinding.Status.Binding.Name).To(Equal(cfServiceInstance.Name))
				}).Should(Succeed())
			})

			When("the referenced legacy binding secret cannot be found", func() {
				JustBeforeEach(func() {
					Expect(adminClient.Delete(ctx, credentialsSecret)).To(Succeed())
				})

				It("sets credentials secret not available condition", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfServiceBinding), cfServiceBinding)).To(Succeed())

						g.Expect(cfServiceBinding.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
							"Type":               Equal(services.BindingSecretAvailableCondition),
							"Status":             Equal(metav1.ConditionFalse),
							"Reason":             Equal("FailedReconcilingCredentialsSecret"),
							"ObservedGeneration": Equal(cfServiceBinding.Generation),
						})))
					}).Should(Succeed())
				})
			})
		})
	})
})
