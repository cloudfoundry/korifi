package integration_test

import (
	"context"

	. "github.com/onsi/gomega/gstruct"
	"sigs.k8s.io/controller-runtime/pkg/client"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFServiceInstance", func() {
	var (
		namespace         *corev1.Namespace
		secret            *corev1.Secret
		cfServiceInstance *korifiv1alpha1.CFServiceInstance
	)

	BeforeEach(func() {
		namespace = BuildNamespaceObject(GenerateGUID())
		Expect(
			k8sClient.Create(context.Background(), namespace),
		).To(Succeed())

		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secret-name",
				Namespace: namespace.Name,
			},
			StringData: map[string]string{"foo": "bar"},
		}

		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		cfServiceInstance = &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-instance-guid",
				Namespace: namespace.Name,
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "service-instance-name",
				Type:        "user-provided",
				Tags:        []string{},
			},
		}
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
	})

	Describe("Create", func() {
		JustBeforeEach(func() {
			Expect(k8sClient.Create(context.Background(), cfServiceInstance)).To(Succeed())
		})

		It("adds a finalizer", func() {
			Eventually(func(g Gomega) {
				updatedCFServiceInstance := new(korifiv1alpha1.CFServiceInstance)
				serviceInstanceNamespacedName := client.ObjectKeyFromObject(cfServiceInstance)
				err := k8sClient.Get(context.Background(), serviceInstanceNamespacedName, updatedCFServiceInstance)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(updatedCFServiceInstance.ObjectMeta.Finalizers).To(ConsistOf("cfServiceInstance.korifi.cloudfoundry.org"))
			}).Should(Succeed())
		})

		When("the secret exists", func() {
			BeforeEach(func() {
				cfServiceInstance.Spec.SecretName = secret.Name
			})

			It("sets the BindingSecretAvailable condition to true in the CFServiceInstance status", func() {
				Eventually(func(g Gomega) {
					updatedCFServiceInstance := new(korifiv1alpha1.CFServiceInstance)
					serviceInstanceNamespacedName := client.ObjectKeyFromObject(cfServiceInstance)
					err := k8sClient.Get(context.Background(), serviceInstanceNamespacedName, updatedCFServiceInstance)
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(updatedCFServiceInstance.Status.Binding.Name).To(Equal("secret-name"))
					g.Expect(updatedCFServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
						"Type":    Equal("BindingSecretAvailable"),
						"Status":  Equal(metav1.ConditionTrue),
						"Reason":  Equal("SecretFound"),
						"Message": Equal(""),
					})))
				}).Should(Succeed())
			})
		})

		When("the referenced secret does not exist", func() {
			BeforeEach(func() {
				cfServiceInstance.Spec.SecretName = "other-secret-name"
			})

			It("sets the BindingSecretAvailable condition to false in the CFServiceInstance status", func() {
				Eventually(func(g Gomega) {
					updatedCFServiceInstance := new(korifiv1alpha1.CFServiceInstance)
					serviceInstanceNamespacedName := client.ObjectKeyFromObject(cfServiceInstance)
					err := k8sClient.Get(context.Background(), serviceInstanceNamespacedName, updatedCFServiceInstance)
					g.Expect(err).NotTo(HaveOccurred())

					g.Expect(updatedCFServiceInstance.Status.Binding).To(BeZero())
					g.Expect(updatedCFServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
						"Type":    Equal("BindingSecretAvailable"),
						"Status":  Equal(metav1.ConditionFalse),
						"Reason":  Equal("SecretNotFound"),
						"Message": Equal("Binding secret does not exist"),
					})))
				}).Should(Succeed())
			})

			When("the referenced secret is created afterwards", func() {
				BeforeEach(func() {
					Expect(k8sClient.Create(context.Background(), &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "other-secret-name",
							Namespace: namespace.Name,
						},
					})).To(Succeed())
				})

				It("sets the BindingSecretAvailable condition to true in the CFServiceInstance status", func() {
					Eventually(func(g Gomega) {
						updatedCFServiceInstance := new(korifiv1alpha1.CFServiceInstance)
						serviceInstanceNamespacedName := client.ObjectKeyFromObject(cfServiceInstance)
						err := k8sClient.Get(context.Background(), serviceInstanceNamespacedName, updatedCFServiceInstance)
						g.Expect(err).NotTo(HaveOccurred())

						g.Expect(updatedCFServiceInstance.Status.Binding.Name).To(Equal("other-secret-name"))
						g.Expect(updatedCFServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
							"Type":    Equal("BindingSecretAvailable"),
							"Status":  Equal(metav1.ConditionTrue),
							"Reason":  Equal("SecretFound"),
							"Message": Equal(""),
						})))
					}).Should(Succeed())
				})
			})
		})
	})

	Describe("Delete", func() {
		BeforeEach(func() {
			cfServiceInstance.Spec.SecretName = secret.Name
			Expect(k8sClient.Create(context.Background(), cfServiceInstance)).To(Succeed())

			Eventually(func(g Gomega) {
				updatedCFServiceInstance := new(korifiv1alpha1.CFServiceInstance)
				serviceInstanceNamespacedName := client.ObjectKeyFromObject(cfServiceInstance)
				err := k8sClient.Get(context.Background(), serviceInstanceNamespacedName, updatedCFServiceInstance)
				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(updatedCFServiceInstance.ObjectMeta.Finalizers).To(ConsistOf("cfServiceInstance.korifi.cloudfoundry.org"))
			}).Should(Succeed())

			cfServiceBinding := &korifiv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      GenerateGUID(),
					Namespace: namespace.Name,
				},
				Spec: korifiv1alpha1.CFServiceBindingSpec{
					Service: corev1.ObjectReference{
						Kind:       "ServiceInstance",
						Name:       cfServiceInstance.Name,
						APIVersion: "korifi.cloudfoundry.org/v1alpha1",
					},
					AppRef: corev1.LocalObjectReference{
						Name: "",
					},
				},
			}
			Expect(k8sClient.Create(context.Background(), cfServiceBinding)).To(Succeed())
		})

		JustBeforeEach(func() {
			Expect(k8sClient.Delete(context.Background(), cfServiceInstance)).To(Succeed())
		})

		It("deletes associated ServiceBindings", func() {
			Eventually(func(g Gomega) {
				cfServiceBindingList := new(korifiv1alpha1.CFServiceBindingList)
				g.Expect(k8sClient.List(context.Background(), cfServiceBindingList, client.InNamespace(namespace.Name))).To(Succeed())

				g.Expect(cfServiceBindingList.Items).To(BeEmpty())
			}).Should(Succeed())
		})
	})
})
