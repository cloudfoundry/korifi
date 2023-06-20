package services_test

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
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
		secret               *corev1.Secret
		secretType           string
		secretProvider       string
		cfServiceBinding     *korifiv1alpha1.CFServiceBinding
		cfServiceBindingGUID string
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

		Expect(k8s.Patch(context.Background(), k8sClient, desiredCFApp, func() {
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

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
	})

	JustBeforeEach(func() {
		Expect(
			k8sClient.Create(context.Background(), cfServiceBinding),
		).To(Succeed())
	})

	It("makes the service instance owner of the service binding", func() {
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceBinding), cfServiceBinding)).To(Succeed())
			g.Expect(cfServiceBinding.GetOwnerReferences()).To(ConsistOf(HaveField("Name", cfServiceInstance.Name)))
		}).Should(Succeed())
	})

	It("resolves the secretName and updates the CFServiceBinding status", func() {
		Eventually(func(g Gomega) {
			updatedCFServiceBinding := new(korifiv1alpha1.CFServiceBinding)
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceBinding), updatedCFServiceBinding)).To(Succeed())
			g.Expect(updatedCFServiceBinding.Status).To(MatchFields(IgnoreExtras, Fields{
				"Binding": MatchFields(IgnoreExtras, Fields{"Name": Equal(secret.Name)}),
				"Conditions": ContainElement(MatchFields(IgnoreExtras, Fields{
					"Type":    Equal("BindingSecretAvailable"),
					"Status":  Equal(metav1.ConditionTrue),
					"Reason":  Equal("SecretFound"),
					"Message": Equal(""),
				})),
			}))
		}).Should(Succeed())
	})

	It("creates a servicebinding.io ServiceBinding", func() {
		Eventually(func(g Gomega) {
			sbServiceBinding := servicebindingv1beta1.ServiceBinding{}
			g.Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: fmt.Sprintf("cf-binding-%s", cfServiceBindingGUID), Namespace: namespace.Name}, &sbServiceBinding)).To(Succeed())
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
		}).Should(Succeed())
	})

	It("sets the ObservedGeneration status field", func() {
		Eventually(func(g Gomega) {
			g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceBinding), cfServiceBinding)).To(Succeed())
			g.Expect(cfServiceBinding.Status.ObservedGeneration).To(Equal(cfServiceBinding.Generation))
		}).Should(Succeed())
	})

	It("writes a log message", func() {
		Eventually(logOutput).Should(gbytes.Say("set observed generation"))
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
				g.Expect(k8sClient.Get(context.Background(), types.NamespacedName{Name: fmt.Sprintf("cf-binding-%s", cfServiceBindingGUID), Namespace: namespace.Name}, &sbServiceBinding)).To(Succeed())
				g.Expect(sbServiceBinding).To(MatchFields(IgnoreExtras, Fields{
					"Spec": MatchFields(IgnoreExtras, Fields{
						"Name": Equal(bindingName),
					}),
				}))
			}).Should(Succeed())
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
			Eventually(func(g Gomega) {
				updatedCFServiceBinding := new(korifiv1alpha1.CFServiceBinding)
				g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceBinding), updatedCFServiceBinding)).To(Succeed())
				g.Expect(updatedCFServiceBinding.Status).To(MatchFields(IgnoreExtras, Fields{
					"Binding": MatchFields(IgnoreExtras, Fields{"Name": Equal("")}),
					"Conditions": ContainElement(MatchFields(IgnoreExtras, Fields{
						"Type":    Equal("BindingSecretAvailable"),
						"Status":  Equal(metav1.ConditionFalse),
						"Reason":  Equal("SecretNotFound"),
						"Message": Equal("Binding secret does not exist"),
					})),
				}))
			}).Should(Succeed())
		})

		When("the referenced secret is created afterwards", func() {
			JustBeforeEach(func() {
				Eventually(func(g Gomega) {
					updatedCFServiceBinding := new(korifiv1alpha1.CFServiceBinding)
					g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceBinding), updatedCFServiceBinding)).To(Succeed())
					g.Expect(updatedCFServiceBinding.Status).To(MatchFields(IgnoreExtras, Fields{
						"Conditions": ContainElement(MatchFields(IgnoreExtras, Fields{
							"Type":   Equal("BindingSecretAvailable"),
							"Status": Equal(metav1.ConditionFalse),
						})),
					}))
				}).Should(Succeed())

				Expect(k8sClient.Create(context.Background(), otherSecret)).To(Succeed())
			})

			It("resolves the secretName and updates the CFServiceBinding status", func() {
				Eventually(func(g Gomega) {
					updatedCFServiceBinding := new(korifiv1alpha1.CFServiceBinding)
					g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceBinding), updatedCFServiceBinding)).To(Succeed())
					g.Expect(updatedCFServiceBinding.Status).To(MatchFields(IgnoreExtras, Fields{
						"Binding": MatchFields(IgnoreExtras, Fields{"Name": Equal(otherSecret.Name)}),
						"Conditions": ContainElement(MatchFields(IgnoreExtras, Fields{
							"Type":    Equal("BindingSecretAvailable"),
							"Status":  Equal(metav1.ConditionTrue),
							"Reason":  Equal("SecretFound"),
							"Message": Equal(""),
						})),
					}))
				}).Should(Succeed())
			})
		})
	})
})
