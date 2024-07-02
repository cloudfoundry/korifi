package brokers_test

import (
	"github.com/google/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFServiceBroker", func() {
	var (
		testNamespace string
		broker        *korifiv1alpha1.CFServiceBroker
	)

	BeforeEach(func() {
		testNamespace = uuid.NewString()
		Expect(adminClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		})).To(Succeed())

		broker = &korifiv1alpha1.CFServiceBroker{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      uuid.NewString(),
			},
		}
		Expect(adminClient.Create(ctx, broker)).To(Succeed())
	})

	It("sets the ObservedGeneration status field", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(broker), broker)).To(Succeed())
			g.Expect(broker.Status.ObservedGeneration).To(Equal(broker.Generation))
		}).Should(Succeed())
	})

	It("sets the Ready condition to false", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(broker), broker)).To(Succeed())
			g.Expect(broker.Status.Conditions).To(ContainElement(SatisfyAll(
				HasType(Equal(korifiv1alpha1.StatusConditionReady)),
				HasStatus(Equal(metav1.ConditionFalse)),
				HasReason(Equal("CredentialsSecretNotAvailable")),
			)))
		}).Should(Succeed())
	})

	Describe("credentials secret", func() {
		var credentialsSecret *corev1.Secret

		BeforeEach(func() {
			credentialsSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					korifiv1alpha1.CredentialsSecretKey: []byte(`{"username": "broker-user", "password": "broker-password"}`),
				},
			}
			Expect(adminClient.Create(ctx, credentialsSecret)).To(Succeed())
			Expect(k8s.PatchResource(ctx, adminClient, broker, func() {
				broker.Spec.Credentials.Name = credentialsSecret.Name
			})).To(Succeed())
		})

		It("sets the Ready condition to true", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(broker), broker)).To(Succeed())
				g.Expect(broker.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionTrue)),
				)))
			}).Should(Succeed())
		})

		It("sets the credentials secret observed version", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(broker), broker)).To(Succeed())
				g.Expect(broker.Status.CredentialsObservedVersion).NotTo(BeEmpty())
			}).Should(Succeed())
		})

		When("the credentials secret data does not have the credentials key", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, credentialsSecret, func() {
					credentialsSecret.Data = map[string][]byte{}
				})).To(Succeed())
			})

			It("sets the ready condition to false", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(broker), broker)).To(Succeed())
					g.Expect(broker.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasReason(Equal("SecretInvalid")),
					)))
				}).Should(Succeed())
			})
		})

		When("the credentials secret data does not have username", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, credentialsSecret, func() {
					credentialsSecret.Data = map[string][]byte{
						korifiv1alpha1.CredentialsSecretKey: []byte(`{ "password": "broker-password"}`),
					}
				})).To(Succeed())
			})

			It("sets the ready condition to false", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(broker), broker)).To(Succeed())
					g.Expect(broker.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasReason(Equal("SecretInvalid")),
					)))
				}).Should(Succeed())
			})
		})

		When("the credentials secret data does not have password", func() {
			BeforeEach(func() {
				Expect(k8s.PatchResource(ctx, adminClient, credentialsSecret, func() {
					credentialsSecret.Data = map[string][]byte{
						korifiv1alpha1.CredentialsSecretKey: []byte(`{ "username": "broker-username"}`),
					}
				})).To(Succeed())
			})

			It("sets the ready condition to false", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(broker), broker)).To(Succeed())
					g.Expect(broker.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasReason(Equal("SecretInvalid")),
					)))
				}).Should(Succeed())
			})
		})

		When("the credentials secret is reconciled", func() {
			var credentialsObservedVersion string

			BeforeEach(func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(broker), broker)).To(Succeed())
					g.Expect(meta.IsStatusConditionTrue(broker.Status.Conditions, korifiv1alpha1.StatusConditionReady)).To(BeTrue())
				}).Should(Succeed())
				credentialsObservedVersion = broker.Status.CredentialsObservedVersion
			})

			When("the credentials secret changes", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, adminClient, credentialsSecret, func() {
						credentialsSecret.StringData = map[string]string{"f": "b"}
					})).To(Succeed())
				})

				It("updates the credentials secret observed version", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(broker), broker)).To(Succeed())
						g.Expect(broker.Status.CredentialsObservedVersion).NotTo(Equal(credentialsObservedVersion))
					}).Should(Succeed())
				})
			})

			When("the credentials secret gets deleted", func() {
				BeforeEach(func() {
					Expect(adminClient.Delete(ctx, credentialsSecret)).To(Succeed())
				})

				It("does not change the credentials secret observed version", func() {
					Consistently(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(broker), broker)).To(Succeed())
						g.Expect(broker.Status.CredentialsObservedVersion).To(Equal(credentialsObservedVersion))
					}).Should(Succeed())
				})
			})
		})
	})
})
