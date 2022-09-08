package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/env"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	servicebindingv1beta1 "github.com/servicebinding/service-binding-controller/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFServiceBinding", func() {
	var (
		namespace                             *corev1.Namespace
		cfAppGUID                             string
		desiredCFApp                          *korifiv1alpha1.CFApp
		cfServiceInstance, cfServiceInstance2 *korifiv1alpha1.CFServiceInstance
		secret, secret2                       *corev1.Secret
		secretType, secretType2               string
		secretProvider, secretProvider2       string
	)

	BeforeEach(func() {
		namespace = BuildNamespaceObject(GenerateGUID())
		Expect(
			k8sClient.Create(context.Background(), namespace),
		).To(Succeed())

		cfAppGUID = GenerateGUID()
		desiredCFApp = BuildCFAppCRObject(cfAppGUID, namespace.Name)
		Expect(
			k8sClient.Create(context.Background(), desiredCFApp),
		).To(Succeed())

		vcapServicesData := map[string]string{
			"VCAP_SERVICES": "{}",
		}
		vcapSecretName := cfAppGUID + "vcap-services"
		vcapServicesSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vcapSecretName,
				Namespace: namespace.Name,
			},
			StringData: vcapServicesData,
		}
		Expect(
			k8sClient.Create(context.Background(), vcapServicesSecret),
		).To(Succeed())

		Expect(k8s.PatchStatus(context.Background(), k8sClient, desiredCFApp, func() {
			desiredCFApp.Status.ObservedDesiredState = korifiv1alpha1.StoppedState
			desiredCFApp.Status.VCAPServicesSecretName = vcapSecretName
		}, metav1.Condition{
			Type:   "Ready",
			Status: metav1.ConditionTrue,
			Reason: "testing",
		})).To(Succeed())

		secretType = "mongodb"
		secretProvider = "cloud-aws"
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-instance-secret",
				Namespace: namespace.Name,
			},
			StringData: map[string]string{
				"type":     secretType,
				"provider": secretProvider,
			},
		}
		Expect(
			k8sClient.Create(context.Background(), secret),
		).To(Succeed())

		cfServiceInstance = &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-instance-guid",
				Namespace: namespace.Name,
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "mongodb-service-instance-name",
				SecretName:  secret.Name,
				Type:        "user-provided",
				Tags:        []string{},
			},
		}
		Expect(
			k8sClient.Create(context.Background(), cfServiceInstance),
		).To(Succeed())

		secretType2 = "dynamodb"
		secretProvider2 = "cloud-aws"
		secret2 = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-instance-secret-2",
				Namespace: namespace.Name,
			},
			StringData: map[string]string{
				"type":     secretType2,
				"provider": secretProvider2,
			},
		}
		Expect(
			k8sClient.Create(context.Background(), secret2),
		).To(Succeed())

		cfServiceInstance2 = &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-instance-guid-2",
				Namespace: namespace.Name,
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "dyanamodb-service-instance-name",
				SecretName:  secret2.Name,
				Type:        "user-provided",
				Tags:        []string{},
			},
		}
		Expect(
			k8sClient.Create(context.Background(), cfServiceInstance2),
		).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
	})

	When("a new CFServiceBinding is Created", func() {
		var (
			cfServiceBinding     *korifiv1alpha1.CFServiceBinding
			cfServiceBindingGUID string
		)
		BeforeEach(func() {
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
			Expect(
				k8sClient.Create(context.Background(), cfServiceBinding),
			).To(Succeed())
		})

		It("eventually resolves the secretName and updates the CFServiceBinding status", func() {
			Eventually(func(g Gomega) korifiv1alpha1.CFServiceBindingStatus {
				updatedCFServiceBinding := new(korifiv1alpha1.CFServiceBinding)
				g.Expect(
					k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceBinding), updatedCFServiceBinding),
				).To(Succeed())

				return updatedCFServiceBinding.Status
			}).Should(MatchFields(IgnoreExtras, Fields{
				"Binding": MatchFields(IgnoreExtras, Fields{"Name": Equal(secret.Name)}),
				"Conditions": ContainElement(MatchFields(IgnoreExtras, Fields{
					"Type":    Equal("BindingSecretAvailable"),
					"Status":  Equal(metav1.ConditionTrue),
					"Reason":  Equal("SecretFound"),
					"Message": Equal(""),
				})),
			}))
		})

		It("creates a servicebinding.io ServiceBinding", func() {
			Eventually(func() (servicebindingv1beta1.ServiceBinding, error) {
				sbServiceBinding := servicebindingv1beta1.ServiceBinding{}
				err := k8sClient.Get(
					context.Background(),
					types.NamespacedName{Name: fmt.Sprintf("cf-binding-%s", cfServiceBindingGUID), Namespace: namespace.Name}, &sbServiceBinding)
				return sbServiceBinding, err
			}).Should(MatchFields(IgnoreExtras, Fields{
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
					"Name":     Equal(secret.Name),
					"Type":     Equal(secretType),
					"Provider": Equal(secretProvider),
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
		})

		It("eventually reconciles to set the owner reference on the CFServiceBinding", func() {
			Eventually(func() []metav1.OwnerReference {
				var createdCFServiceBinding korifiv1alpha1.CFServiceBinding
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfServiceBindingGUID, Namespace: namespace.Name}, &createdCFServiceBinding)
				if err != nil {
					return nil
				}
				return createdCFServiceBinding.GetOwnerReferences()
			}).Should(ConsistOf(metav1.OwnerReference{
				APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				Kind:       "CFApp",
				Name:       desiredCFApp.Name,
				UID:        desiredCFApp.UID,
			}))
		})

		It("sets the `cfServiceBinding.korifi.cloudfoundry.org` finalizer", func() {
			Eventually(func(g Gomega) []string {
				updatedCFServiceBinding := new(korifiv1alpha1.CFServiceBinding)
				g.Expect(
					k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceBinding), updatedCFServiceBinding),
				).To(Succeed())
				return updatedCFServiceBinding.ObjectMeta.Finalizers
			}).Should(ConsistOf([]string{
				"cfServiceBinding.korifi.cloudfoundry.org",
			}))
		})

		When("multiple CFServiceBindings exist for the same CFApp", func() {
			var (
				cfServiceBinding2     *korifiv1alpha1.CFServiceBinding
				cfServiceBindingGUID2 string
			)
			BeforeEach(func() {
				cfServiceBindingGUID2 = GenerateGUID()
				cfServiceBinding2 = &korifiv1alpha1.CFServiceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfServiceBindingGUID2,
						Namespace: namespace.Name,
					},
					Spec: korifiv1alpha1.CFServiceBindingSpec{
						Service: corev1.ObjectReference{
							Kind:       "ServiceInstance",
							Name:       cfServiceInstance2.Name,
							APIVersion: "korifi.cloudfoundry.org/v1alpha1",
						},
						AppRef: corev1.LocalObjectReference{
							Name: cfAppGUID,
						},
					},
				}
				Expect(
					k8sClient.Create(context.Background(), cfServiceBinding2),
				).To(Succeed())
			})

			It("eventually updates the vcap services secret of the referenced cf app", func() {
				vcapServicesSecretLookupKey := types.NamespacedName{Name: desiredCFApp.Status.VCAPServicesSecretName, Namespace: namespace.Name}
				updatedSecret := new(corev1.Secret)
				Eventually(func(g Gomega) []env.ServiceDetails {
					g.Expect(
						k8sClient.Get(context.Background(), vcapServicesSecretLookupKey, updatedSecret),
					).To(Succeed())
					return getUserProvidedServiceDetails(updatedSecret.Data["VCAP_SERVICES"])
				}).WithTimeout(time.Second * 5).Should(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name":         Equal(cfServiceInstance.Spec.DisplayName),
						"InstanceGUID": Equal(cfServiceInstance.Name),
						"InstanceName": Equal(cfServiceInstance.Spec.DisplayName),
						"BindingGUID":  Equal(cfServiceBindingGUID),
						"Credentials": SatisfyAll(
							HaveKeyWithValue("provider", secretProvider),
							HaveKeyWithValue("type", secretType),
						),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name":         Equal(cfServiceInstance2.Spec.DisplayName),
						"InstanceGUID": Equal(cfServiceInstance2.Name),
						"InstanceName": Equal(cfServiceInstance2.Spec.DisplayName),
						"BindingGUID":  Equal(cfServiceBindingGUID2),
						"Credentials": SatisfyAll(
							HaveKeyWithValue("provider", secretProvider2),
							HaveKeyWithValue("type", secretType2),
						),
					}),
				))
			})
		})

		When("the referenced secret does not exist", func() {
			var otherSecret *corev1.Secret

			BeforeEach(func() {
				ctx := context.Background()
				otherSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-secret-name",
						Namespace: namespace.Name,
					},
				}
				instance := &korifiv1alpha1.CFServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-service-instance-guid",
						Namespace: namespace.Name,
					},
					Spec: korifiv1alpha1.CFServiceInstanceSpec{
						DisplayName: "other-service-instance-name",
						SecretName:  otherSecret.Name,
						Type:        "user-provided",
						Tags:        []string{},
					},
				}
				Expect(
					k8sClient.Create(ctx, instance),
				).To(Succeed())

				cfServiceBinding.Spec.Service.Name = instance.Name
			})

			It("updates the CFServiceBinding status", func() {
				Eventually(func(g Gomega) korifiv1alpha1.CFServiceBindingStatus {
					updatedCFServiceBinding := new(korifiv1alpha1.CFServiceBinding)
					g.Expect(
						k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceBinding), updatedCFServiceBinding),
					).To(Succeed())

					return updatedCFServiceBinding.Status
				}).Should(MatchFields(IgnoreExtras, Fields{
					"Binding": MatchFields(IgnoreExtras, Fields{"Name": Equal("")}),
					"Conditions": ContainElement(MatchFields(IgnoreExtras, Fields{
						"Type":    Equal("BindingSecretAvailable"),
						"Status":  Equal(metav1.ConditionFalse),
						"Reason":  Equal("SecretNotFound"),
						"Message": Equal("Binding secret does not exist"),
					})),
				}))
			})

			When("the referenced secret is created afterwards", func() {
				JustBeforeEach(func() {
					time.Sleep(100 * time.Millisecond)
					Expect(
						k8sClient.Create(context.Background(), otherSecret),
					).To(Succeed())
				})

				It("eventually resolves the secretName and updates the CFServiceBinding status", func() {
					Eventually(func(g Gomega) korifiv1alpha1.CFServiceBindingStatus {
						updatedCFServiceBinding := new(korifiv1alpha1.CFServiceBinding)
						g.Expect(
							k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceBinding), updatedCFServiceBinding),
						).To(Succeed())

						return updatedCFServiceBinding.Status
					}).Should(MatchFields(IgnoreExtras, Fields{
						"Binding": MatchFields(IgnoreExtras, Fields{"Name": Equal(otherSecret.Name)}),
						"Conditions": ContainElement(MatchFields(IgnoreExtras, Fields{
							"Type":    Equal("BindingSecretAvailable"),
							"Status":  Equal(metav1.ConditionTrue),
							"Reason":  Equal("SecretFound"),
							"Message": Equal(""),
						})),
					}))
				})
			})
		})
	})

	When("a CFServiceBinding is deleted", func() {
		var (
			cfServiceBinding      *korifiv1alpha1.CFServiceBinding
			cfServiceBindingGUID  string
			cfServiceBinding2     *korifiv1alpha1.CFServiceBinding
			cfServiceBindingGUID2 string
		)
		BeforeEach(func() {
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
			Expect(k8sClient.Create(context.Background(), cfServiceBinding)).To(Succeed())

			cfServiceBindingGUID2 = GenerateGUID()
			cfServiceBinding2 = &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfServiceBindingGUID2,
					Namespace: namespace.Name,
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Service: corev1.ObjectReference{
						Kind:       "ServiceInstance",
						Name:       cfServiceInstance2.Name,
						APIVersion: "korifi.cloudfoundry.org/v1alpha1",
					},
					AppRef: corev1.LocalObjectReference{
						Name: cfAppGUID,
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), cfServiceBinding2)).To(Succeed())
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Delete(context.Background(), cfServiceBinding)).To(Succeed())
		})

		It("removes the instance secret from the vcap services secret", func() {
			vcapServicesSecretLookupKey := types.NamespacedName{Name: desiredCFApp.Status.VCAPServicesSecretName, Namespace: namespace.Name}
			updatedSecret := new(corev1.Secret)
			Eventually(func(g Gomega) []env.ServiceDetails {
				g.Expect(
					k8sClient.Get(context.Background(), vcapServicesSecretLookupKey, updatedSecret),
				).To(Succeed())
				return getUserProvidedServiceDetails(updatedSecret.Data["VCAP_SERVICES"])
			}).Should(SatisfyAll(
				Not(ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"Name":         Equal(cfServiceInstance.Spec.DisplayName),
						"InstanceGUID": Equal(cfServiceInstance.Name),
						"InstanceName": Equal(cfServiceInstance.Spec.DisplayName),
						"BindingGUID":  Equal(cfServiceBindingGUID),
						"Credentials": SatisfyAll(
							HaveKeyWithValue("provider", secretProvider),
							HaveKeyWithValue("type", secretType),
						),
					}),
				)),
				ContainElement(
					MatchFields(IgnoreExtras, Fields{
						"Name":         Equal(cfServiceInstance2.Spec.DisplayName),
						"InstanceGUID": Equal(cfServiceInstance2.Name),
						"InstanceName": Equal(cfServiceInstance2.Spec.DisplayName),
						"BindingGUID":  Equal(cfServiceBindingGUID2),
						"Credentials": SatisfyAll(
							HaveKeyWithValue("provider", secretProvider2),
							HaveKeyWithValue("type", secretType2),
						),
					}),
				),
			))
		})
	})
})

type tempServicesDetails struct {
	UserProvided []env.ServiceDetails `json:"user-provided,omitempty"`
}

func getUserProvidedServiceDetails(vcapServicesData []byte) []env.ServiceDetails {
	vcapServicesDetails := tempServicesDetails{}
	_ = json.Unmarshal(vcapServicesData, &vcapServicesDetails)
	return vcapServicesDetails.UserProvided
}
