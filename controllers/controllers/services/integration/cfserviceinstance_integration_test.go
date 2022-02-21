package integration_test

import (
	"context"
	"time"

	. "github.com/onsi/gomega/gstruct"

	"k8s.io/apimachinery/pkg/types"

	servicesv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/services/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFServiceInstance", func() {
	var namespace *corev1.Namespace

	BeforeEach(func() {
		namespace = BuildNamespaceObject(GenerateGUID())
		Expect(
			k8sClient.Create(context.Background(), namespace),
		).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
	})

	When("a new CFServiceInstance is Created", func() {
		var (
			secretData        map[string]string
			secret            *corev1.Secret
			cfServiceInstance *servicesv1alpha1.CFServiceInstance
		)
		BeforeEach(func() {
			ctx := context.Background()

			secretData = map[string]string{
				"foo": "bar",
			}
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-name",
					Namespace: namespace.Name,
				},
				StringData: secretData,
			}
			Expect(
				k8sClient.Create(ctx, secret),
			).To(Succeed())

			cfServiceInstance = &servicesv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "service-instance-guid",
					Namespace: namespace.Name,
				},
				Spec: servicesv1alpha1.CFServiceInstanceSpec{
					Name:       "service-instance-name",
					SecretName: secret.Name,
					Type:       "user-provided",
					Tags:       []string{},
				},
			}
		})

		JustBeforeEach(func() {
			Expect(
				k8sClient.Create(context.Background(), cfServiceInstance),
			).To(Succeed())
		})

		When("and the secret exists", func() {
			BeforeEach(func() {
				cfServiceInstance.Spec.SecretName = secret.Name
			})

			It("eventually resolves the secretName and updates the CFServiceInstance status", func() {
				updatedCFServiceInstance := new(servicesv1alpha1.CFServiceInstance)
				Eventually(func() string {
					Expect(
						k8sClient.Get(context.Background(), types.NamespacedName{Name: cfServiceInstance.Name, Namespace: cfServiceInstance.Namespace}, updatedCFServiceInstance),
					).To(Succeed())

					return updatedCFServiceInstance.Status.Binding.Name
				}).ShouldNot(BeEmpty())

				Expect(updatedCFServiceInstance.Status.Binding.Name).To(Equal(updatedCFServiceInstance.Spec.SecretName))
				Expect(updatedCFServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
					"Type":    Equal("BindingSecretAvailable"),
					"Status":  Equal(metav1.ConditionTrue),
					"Reason":  Equal("SecretFound"),
					"Message": Equal(""),
				})))
			})
		})

		When("and the referenced secret does not exist", func() {
			var otherSecret *corev1.Secret

			BeforeEach(func() {
				otherSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-secret-name",
						Namespace: namespace.Name,
					},
				}
				cfServiceInstance.Spec.SecretName = otherSecret.Name
			})

			It("updates the CFServiceInstance status", func() {
				updatedCFServiceInstance := new(servicesv1alpha1.CFServiceInstance)
				Eventually(func() servicesv1alpha1.CFServiceInstanceStatus {
					Expect(
						k8sClient.Get(context.Background(), types.NamespacedName{Name: cfServiceInstance.Name, Namespace: cfServiceInstance.Namespace}, updatedCFServiceInstance),
					).To(Succeed())

					return updatedCFServiceInstance.Status
				}).ShouldNot(Equal(servicesv1alpha1.CFServiceInstanceStatus{}))

				Expect(updatedCFServiceInstance.Status.Binding.Name).To(Equal(""))
				Expect(updatedCFServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
					"Type":    Equal("BindingSecretAvailable"),
					"Status":  Equal(metav1.ConditionFalse),
					"Reason":  Equal("SecretNotFound"),
					"Message": Equal("Binding secret does not exist"),
				})))
			})

			When("the referenced secret is created afterwards", func() {
				JustBeforeEach(func() {
					time.Sleep(100 * time.Millisecond)
					Expect(
						k8sClient.Create(context.Background(), otherSecret),
					).To(Succeed())
				})

				It("eventually resolves the secretName and updates the CFServiceInstance status", func() {
					updatedCFServiceInstance := new(servicesv1alpha1.CFServiceInstance)
					Eventually(func() string {
						Expect(
							k8sClient.Get(context.Background(), types.NamespacedName{Name: cfServiceInstance.Name, Namespace: cfServiceInstance.Namespace}, updatedCFServiceInstance),
						).To(Succeed())

						return updatedCFServiceInstance.Status.Binding.Name
					}).ShouldNot(BeEmpty())

					Expect(updatedCFServiceInstance.Status.Binding.Name).To(Equal(updatedCFServiceInstance.Spec.SecretName))
					Expect(updatedCFServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
						"Type":    Equal("BindingSecretAvailable"),
						"Status":  Equal(metav1.ConditionTrue),
						"Reason":  Equal("SecretFound"),
						"Message": Equal(""),
					})))
				})
			})
		})
	})
})
