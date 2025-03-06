package upsi_test

import (
	"github.com/google/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFServiceInstance", func() {
	var (
		testNamespace string
		instance      *korifiv1alpha1.CFServiceInstance
	)

	BeforeEach(func() {
		testNamespace = uuid.NewString()
		Expect(adminClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		})).To(Succeed())
	})

	When("the service instance is user-provided", func() {
		BeforeEach(func() {
			instance = &korifiv1alpha1.CFServiceInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: testNamespace,
					Finalizers: []string{
						korifiv1alpha1.CFServiceInstanceFinalizerName,
					},
				},
				Spec: korifiv1alpha1.CFServiceInstanceSpec{
					DisplayName: "service-instance-name",
					Type:        korifiv1alpha1.UserProvidedType,
					Tags:        []string{},
				},
			}
			Expect(adminClient.Create(ctx, instance)).To(Succeed())
		})

		It("sets the ObservedGeneration status field", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(instance.Status.ObservedGeneration).To(Equal(instance.Generation))
			}).Should(Succeed())
		})

		It("sets the CredentialsSecretAvailable condition to false", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
					HasReason(Equal("CredentialsSecretNotAvailable")),
				)))
			}).Should(Succeed())
		})

		When("the credentials secret gets created", func() {
			var credentialsSecret *corev1.Secret

			BeforeEach(func() {
				credentialsSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: testNamespace,
					},
					Data: map[string][]byte{
						tools.CredentialsSecretKey: []byte(`{"foo": "bar"}`),
					},
				}
				Expect(adminClient.Create(ctx, credentialsSecret)).To(Succeed())

				Expect(k8s.PatchResource(ctx, adminClient, instance, func() {
					instance.Spec.SecretName = credentialsSecret.Name
				})).To(Succeed())
			})

			It("sets the CredentialsSecretAvailable condition to true", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionTrue)),
					)))
				}).Should(Succeed())
			})

			It("sets the instance credentials secret name and observed version", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(instance.Status.Credentials.Name).To(Equal(instance.Spec.SecretName))
					g.Expect(instance.Status.CredentialsObservedVersion).NotTo(BeEmpty())
				}).Should(Succeed())
			})

			When("the credentials secret is invalid", func() {
				BeforeEach(func() {
					Expect(k8s.PatchResource(ctx, adminClient, credentialsSecret, func() {
						credentialsSecret.Data = map[string][]byte{}
					})).To(Succeed())
				})

				It("sets credentials secret available condition to false", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
						g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
							HasType(Equal(korifiv1alpha1.StatusConditionReady)),
							HasStatus(Equal(metav1.ConditionFalse)),
							HasReason(Equal("SecretInvalid")),
						)))
					}).Should(Succeed())
				})

				It("sets the instance last operation failed state", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
						g.Expect(instance.Status.LastOperation).To(Equal(korifiv1alpha1.LastOperation{
							Type:  "create",
							State: "failed",
						}))
					}).Should(Succeed())
				})
			})

			It("sets the instance last operation succeed state", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(instance.Status.LastOperation).To(Equal(korifiv1alpha1.LastOperation{
						Type:  "create",
						State: "succeeded",
					}))
				}).Should(Succeed())
			})

			When("the credentials secret changes", func() {
				var secretVersion string

				BeforeEach(func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
						g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
							HasType(Equal(korifiv1alpha1.StatusConditionReady)),
							HasStatus(Equal(metav1.ConditionTrue)),
						)))
						secretVersion = instance.Status.CredentialsObservedVersion
					}).Should(Succeed())

					Expect(k8s.Patch(ctx, adminClient, credentialsSecret, func() {
						credentialsSecret.StringData = map[string]string{"f": "b"}
					})).To(Succeed())
				})

				It("updates the credentials secret observed version", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
						g.Expect(instance.Status.CredentialsObservedVersion).NotTo(Equal(secretVersion))
					}).Should(Succeed())
				})

				It("sets the instance last operation update type", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
						g.Expect(instance.Status.LastOperation).To(Equal(korifiv1alpha1.LastOperation{
							Type:  "update",
							State: "succeeded",
						}))
					}).Should(Succeed())
				})
			})

			When("the credentials secret gets deleted", func() {
				var lastObservedVersion string

				BeforeEach(func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
						g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
							HasType(Equal(korifiv1alpha1.StatusConditionReady)),
							HasStatus(Equal(metav1.ConditionTrue)),
						)))
						lastObservedVersion = instance.Status.CredentialsObservedVersion
					}).Should(Succeed())

					Expect(adminClient.Delete(ctx, credentialsSecret)).To(Succeed())
				})

				It("does not change the instance credentials secret name and observed version", func() {
					Consistently(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
						g.Expect(instance.Status.Credentials.Name).To(Equal(credentialsSecret.Name))
						g.Expect(instance.Status.CredentialsObservedVersion).To(Equal(lastObservedVersion))
					}).Should(Succeed())
				})
			})

			When("the instance is deleted", func() {
				JustBeforeEach(func() {
					Expect(adminClient.Delete(ctx, instance)).To(Succeed())
				})

				It("is deleted", func() {
					Eventually(func(g Gomega) {
						err := adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
						g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
					}).Should(Succeed())
				})
			})
		})

		When("the service instance is managed", func() {
			BeforeEach(func() {
				instance = &korifiv1alpha1.CFServiceInstance{
					ObjectMeta: metav1.ObjectMeta{
						Name:      uuid.NewString(),
						Namespace: testNamespace,
					},
					Spec: korifiv1alpha1.CFServiceInstanceSpec{
						DisplayName: "service-instance-name",
						Type:        korifiv1alpha1.ManagedType,
						Tags:        []string{},
					},
				}
				Expect(adminClient.Create(ctx, instance)).To(Succeed())
			})

			It("does not reconcile it", func() {
				Consistently(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(instance.Status).To(BeZero())
				}).Should(Succeed())
			})
		})
	})
})
