package services_test

import (
	"context"

	. "github.com/onsi/gomega/gstruct"
	"sigs.k8s.io/controller-runtime/pkg/client"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"
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
			adminClient.Create(context.Background(), namespace),
		).To(Succeed())

		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secret-name",
				Namespace: namespace.Name,
			},
			StringData: map[string]string{"foo": "bar"},
		}

		Expect(adminClient.Create(ctx, secret)).To(Succeed())

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
		Expect(adminClient.Delete(context.Background(), namespace)).To(Succeed())
	})

	JustBeforeEach(func() {
		Expect(adminClient.Create(context.Background(), cfServiceInstance)).To(Succeed())
	})

	It("sets the BindingSecretAvailable condition to true in the CFServiceInstance status", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())

			g.Expect(cfServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Type":    Equal("BindingSecretAvailable"),
				"Status":  Equal(metav1.ConditionTrue),
				"Reason":  Equal("SecretFound"),
				"Message": Equal(""),
			})))
			g.Expect(cfServiceInstance.Status.Credentials.Name).To(Equal(cfServiceInstance.Spec.SecretName))
			g.Expect(cfServiceInstance.Status.CredentialsObservedVersion).NotTo(BeEmpty())
		}).Should(Succeed())
	})

	It("sets the ObservedGeneration status field", func() {
		Eventually(func(g Gomega) {
			updatedCFServiceInstance := new(korifiv1alpha1.CFServiceInstance)
			serviceInstanceNamespacedName := client.ObjectKeyFromObject(cfServiceInstance)
			g.Expect(adminClient.Get(context.Background(), serviceInstanceNamespacedName, updatedCFServiceInstance)).To(Succeed())
			g.Expect(updatedCFServiceInstance.Status.ObservedGeneration).To(Equal(cfServiceInstance.Generation))
		}).Should(Succeed())
	})

	When("the credentials secret changes", func() {
		var (
			credentialsSecret *corev1.Secret
			secretVersion     string
		)

		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())
				g.Expect(cfServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
					"Type":    Equal("BindingSecretAvailable"),
					"Status":  Equal(metav1.ConditionTrue),
					"Reason":  Equal("SecretFound"),
					"Message": Equal(""),
				})))
				secretVersion = cfServiceInstance.Status.CredentialsObservedVersion
			}).Should(Succeed())

			credentialsSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cfServiceInstance.Namespace,
					Name:      cfServiceInstance.Spec.SecretName,
				},
			}

			Expect(k8s.Patch(ctx, adminClient, credentialsSecret, func() {
				credentialsSecret.StringData = map[string]string{"f": "b"}
			})).To(Succeed())
		})

		It("updates the credentials secret observed version", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())
				g.Expect(cfServiceInstance.Status.CredentialsObservedVersion).NotTo(Equal(secretVersion))
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
				g.Expect(adminClient.Get(context.Background(), serviceInstanceNamespacedName, updatedCFServiceInstance)).To(Succeed())

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
				Expect(adminClient.Create(context.Background(), &corev1.Secret{
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
					g.Expect(adminClient.Get(context.Background(), serviceInstanceNamespacedName, updatedCFServiceInstance)).To(Succeed())

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
