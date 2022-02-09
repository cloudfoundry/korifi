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

var _ = Describe("CFServiceBinding", func() {
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

	When("a new CFServiceBinding is Created", func() {
		var (
			secretData        map[string]string
			secret            *corev1.Secret
			cfServiceInstance *servicesv1alpha1.CFServiceInstance
			cfServiceBinding  *servicesv1alpha1.CFServiceBinding
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
			Expect(
				k8sClient.Create(ctx, cfServiceInstance),
			).To(Succeed())

			cfServiceBinding = &servicesv1alpha1.CFServiceBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      GenerateGUID(),
					Namespace: namespace.Name,
				},
				Spec: servicesv1alpha1.CFServiceBindingSpec{
					Name: "",
					Service: corev1.ObjectReference{
						Kind:       "ServiceInstance",
						Name:       cfServiceInstance.Name,
						APIVersion: "services.cloudfoundry.org/v1alpha1",
					},
					SecretName: "",
					AppRef: corev1.LocalObjectReference{
						Name: "",
					},
				},
			}
		})

		JustBeforeEach(func() {
			Expect(
				k8sClient.Create(context.Background(), cfServiceBinding),
			).To(Succeed())
		})

		When("and the secret exists", func() {
			BeforeEach(func() {
				cfServiceBinding.Spec.SecretName = secret.Name
			})

			It("eventually resolves the secretName and updates the CFServiceBinding status", func() {
				updatedCFServiceBinding := new(servicesv1alpha1.CFServiceBinding)
				Eventually(func() string {
					Expect(
						k8sClient.Get(context.Background(), types.NamespacedName{Name: cfServiceBinding.Name, Namespace: cfServiceBinding.Namespace}, updatedCFServiceBinding),
					).To(Succeed())

					return updatedCFServiceBinding.Status.Binding.Name
				}, defaultEventuallyTimeoutSeconds*time.Second).ShouldNot(BeEmpty())

				Expect(updatedCFServiceBinding.Status.Binding.Name).To(Equal(updatedCFServiceBinding.Spec.SecretName))
				Expect(updatedCFServiceBinding.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
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
				cfServiceBinding.Spec.SecretName = otherSecret.Name
			})

			It("updates the CFServiceBinding status", func() {
				updatedCFServiceBinding := new(servicesv1alpha1.CFServiceBinding)
				Eventually(func() servicesv1alpha1.CFServiceBindingStatus {
					Expect(
						k8sClient.Get(context.Background(), types.NamespacedName{Name: cfServiceBinding.Name, Namespace: cfServiceBinding.Namespace}, updatedCFServiceBinding),
					).To(Succeed())

					return updatedCFServiceBinding.Status
				}, defaultEventuallyTimeoutSeconds*time.Second).ShouldNot(Equal(servicesv1alpha1.CFServiceBindingStatus{}))

				Expect(updatedCFServiceBinding.Status.Binding.Name).To(Equal(""))
				Expect(updatedCFServiceBinding.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
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

				It("eventually resolves the secretName and updates the CFServiceBinding status", func() {
					updatedCFServiceBinding := new(servicesv1alpha1.CFServiceBinding)
					Eventually(func() string {
						Expect(
							k8sClient.Get(context.Background(), types.NamespacedName{Name: cfServiceBinding.Name, Namespace: cfServiceBinding.Namespace}, updatedCFServiceBinding),
						).To(Succeed())

						return updatedCFServiceBinding.Status.Binding.Name
					}, defaultEventuallyTimeoutSeconds*time.Second).ShouldNot(BeEmpty())

					Expect(updatedCFServiceBinding.Status.Binding.Name).To(Equal(updatedCFServiceBinding.Spec.SecretName))
					Expect(updatedCFServiceBinding.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
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
