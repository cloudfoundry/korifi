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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	servicebindingv1beta1 "github.com/servicebinding/service-binding-controller/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("CFServiceBinding", func() {
	var namespace *corev1.Namespace
	var cfAppGUID string
	var desiredCFApp *korifiv1alpha1.CFApp

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

		originalApp := desiredCFApp.DeepCopy()
		desiredCFApp.Status = korifiv1alpha1.CFAppStatus{
			Conditions:             nil,
			ObservedDesiredState:   korifiv1alpha1.StoppedState,
			VCAPServicesSecretName: vcapSecretName,
		}
		meta.SetStatusCondition(&desiredCFApp.Status.Conditions, metav1.Condition{
			Type:   "Ready",
			Status: metav1.ConditionTrue,
			Reason: "testing",
		})
		Expect(
			k8sClient.Status().Patch(context.Background(), desiredCFApp, client.MergeFrom(originalApp)),
		).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
	})

	When("a new CFServiceBinding is Created", func() {
		var (
			secretData           map[string]string
			secret               *corev1.Secret
			cfServiceInstance    *korifiv1alpha1.CFServiceInstance
			cfServiceBinding     *korifiv1alpha1.CFServiceBinding
			cfServiceBindingGUID string
			secretName           string
			secretType           string
			secretProvider       string
		)
		BeforeEach(func() {
			ctx := context.Background()

			secretName = "secret-name"
			secretType = "mongodb"
			secretProvider = "cloud-aws"
			secretData = map[string]string{
				"type":     secretType,
				"provider": secretProvider,
			}
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace.Name,
				},
				StringData: secretData,
			}
			Expect(
				k8sClient.Create(ctx, secret),
			).To(Succeed())

			cfServiceInstance = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service-instance-guid",
					Namespace: namespace.Name,
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					DisplayName: "service-instance-name",
					SecretName:  secret.Name,
					Type:        "user-provided",
					Tags:        []string{},
				},
			}
			Expect(
				k8sClient.Create(ctx, cfServiceInstance),
			).To(Succeed())

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

		It("eventually updates the vcap services secret of the referenced cf app", func() {
			ctx := context.Background()
			vcapServicesSecretLookupKey := types.NamespacedName{Name: desiredCFApp.Status.VCAPServicesSecretName, Namespace: namespace.Name}
			updatedSecret := new(corev1.Secret)
			Eventually(func(g Gomega) []byte {
				g.Expect(
					k8sClient.Get(ctx, vcapServicesSecretLookupKey, updatedSecret),
				).To(Succeed())
				return updatedSecret.Data["VCAP_SERVICES"]
			}).WithTimeout(time.Second * 5).Should(MatchJSON(fmt.Sprintf(`{
																					   "user-provided": [{
																					   "label":            "user-provided",
																					   "name":             "service-instance-name",
																					   "tags":             [],
																					   "instance_guid":    "service-instance-guid",
																					   "instance_name":    "service-instance-name",
																					   "binding_guid":     "%[1]s",
																					   "binding_name":     null,
																					   "credentials":      {
																					   "provider": "cloud-aws",
																					   "type": "mongodb"
																					   },
																					   "syslog_drain_url": null,
																					   "volume_mounts": []}
																					   ]}`, cfServiceBindingGUID,
			)))
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

		When("multiple CFServiceBindings exist for the same CFApp", func() {
			var (
				secretData2           map[string]string
				secret2               *corev1.Secret
				cfServiceInstance2    *korifiv1alpha1.CFServiceInstance
				cfServiceBinding2     *korifiv1alpha1.CFServiceBinding
				cfServiceBindingGUID2 string
				secretName2           string
				secretType2           string
				secretProvider2       string
			)
			BeforeEach(func() {
				ctx := context.Background()
				secretName2 = "secret-name-2"
				secretType2 = "dynamodb"
				secretProvider2 = "cloud-aws"
				secretData2 = map[string]string{
					"type":     secretType2,
					"provider": secretProvider2,
				}
				secret2 = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      secretName2,
						Namespace: namespace.Name,
					},
					StringData: secretData2,
				}
				Expect(
					k8sClient.Create(ctx, secret2),
				).To(Succeed())

				cfServiceInstance2 = &korifiv1alpha1.CFServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "service-instance-guid-2",
						Namespace: namespace.Name,
					},
					Spec: korifiv1alpha1.CFServiceInstanceSpec{
						DisplayName: "service-instance-name-2",
						SecretName:  secret2.Name,
						Type:        "user-provided",
						Tags:        []string{},
					},
				}
				Expect(
					k8sClient.Create(ctx, cfServiceInstance2),
				).To(Succeed())

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
				ctx := context.Background()
				vcapServicesSecretLookupKey := types.NamespacedName{Name: desiredCFApp.Status.VCAPServicesSecretName, Namespace: namespace.Name}
				updatedSecret := new(corev1.Secret)
				Eventually(func(g Gomega) []env.ServiceDetails {
					g.Expect(
						k8sClient.Get(ctx, vcapServicesSecretLookupKey, updatedSecret),
					).To(Succeed())
					return getUserProvidedServiceDetails(updatedSecret.Data["VCAP_SERVICES"])
				}).WithTimeout(time.Second * 5).Should(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name":         Equal("service-instance-name"),
						"InstanceGUID": Equal("service-instance-guid"),
						"InstanceName": Equal("service-instance-name"),
						"BindingGUID":  Equal(cfServiceBindingGUID),
						"Credentials": SatisfyAll(
							HaveKeyWithValue("provider", "cloud-aws"),
							HaveKeyWithValue("type", "mongodb"),
						),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Name":         Equal("service-instance-name-2"),
						"InstanceGUID": Equal("service-instance-guid-2"),
						"InstanceName": Equal("service-instance-name-2"),
						"BindingGUID":  Equal(cfServiceBindingGUID2),
						"Credentials": SatisfyAll(
							HaveKeyWithValue("provider", "cloud-aws"),
							HaveKeyWithValue("type", "dynamodb"),
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
})

type tempServicesDetails struct {
	UserProvided []env.ServiceDetails `json:"user-provided,omitempty"`
}

func getUserProvidedServiceDetails(vcapServicesData []byte) []env.ServiceDetails {
	vcapServicesDetails := tempServicesDetails{}
	_ = json.Unmarshal(vcapServicesData, &vcapServicesDetails)
	return vcapServicesDetails.UserProvided
}
