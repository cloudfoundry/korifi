package services_test

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
				SecretName:  secret.Name,
			},
		}
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
	})

	JustBeforeEach(func() {
		Expect(k8sClient.Create(context.Background(), cfServiceInstance)).To(Succeed())
	})

	It("sets the BindingSecretAvailable condition to true in the CFServiceInstance status", func() {
		Eventually(func(g Gomega) {
			updatedCFServiceInstance := new(korifiv1alpha1.CFServiceInstance)
			serviceInstanceNamespacedName := client.ObjectKeyFromObject(cfServiceInstance)
			g.Expect(k8sClient.Get(context.Background(), serviceInstanceNamespacedName, updatedCFServiceInstance)).To(Succeed())

			g.Expect(updatedCFServiceInstance.Status.Binding.Name).To(Equal("secret-name"))
			g.Expect(updatedCFServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Type":    Equal("BindingSecretAvailable"),
				"Status":  Equal(metav1.ConditionTrue),
				"Reason":  Equal("SecretFound"),
				"Message": Equal(""),
			})))
		}).Should(Succeed())
	})

	It("sets the ObservedGeneration status field", func() {
		Eventually(func(g Gomega) {
			updatedCFServiceInstance := new(korifiv1alpha1.CFServiceInstance)
			serviceInstanceNamespacedName := client.ObjectKeyFromObject(cfServiceInstance)
			g.Expect(k8sClient.Get(context.Background(), serviceInstanceNamespacedName, updatedCFServiceInstance)).To(Succeed())
			g.Expect(updatedCFServiceInstance.Status.ObservedGeneration).To(Equal(cfServiceInstance.Generation))
		}).Should(Succeed())
	})

	When("the referenced secret does not exist", func() {
		BeforeEach(func() {
			cfServiceInstance.Spec.SecretName = "other-secret-name"
		})

		It("sets the BindingSecretAvailable condition to false in the CFServiceInstance status", func() {
			Eventually(func(g Gomega) {
				updatedCFServiceInstance := new(korifiv1alpha1.CFServiceInstance)
				serviceInstanceNamespacedName := client.ObjectKeyFromObject(cfServiceInstance)
				g.Expect(k8sClient.Get(context.Background(), serviceInstanceNamespacedName, updatedCFServiceInstance)).To(Succeed())

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
					g.Expect(k8sClient.Get(context.Background(), serviceInstanceNamespacedName, updatedCFServiceInstance)).To(Succeed())

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
