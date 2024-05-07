package instances_test

import (
	"encoding/json"

	"github.com/google/uuid"
	. "github.com/onsi/gomega/gstruct"
	"sigs.k8s.io/controller-runtime/pkg/client"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
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

		instance = &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "service-instance-name",
				Type:        "user-provided",
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
					korifiv1alpha1.CredentialsSecretKey: []byte(`{"foo": "bar"}`),
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
	})

	When("the instance credentials secret is in the 'legacy' format", func() {
		var credentialsSecret *corev1.Secret

		getMigratedSecret := func() *corev1.Secret {
			migratedSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instance.Name + "-migrated",
					Namespace: testNamespace,
				},
			}
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(migratedSecret), migratedSecret)).To(Succeed())
			}).Should(Succeed())

			return migratedSecret
		}

		JustBeforeEach(func() {
			credentialsSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      uuid.NewString(),
					Namespace: testNamespace,
				},
				Type: corev1.SecretType("servicebinding.io/legacy"),
				StringData: map[string]string{
					"foo": "bar",
				},
			}
			Expect(adminClient.Create(ctx, credentialsSecret)).To(Succeed())

			Expect(k8s.PatchResource(ctx, adminClient, instance, func() {
				instance.Spec.SecretName = credentialsSecret.Name
			})).To(Succeed())
		})

		It("creates a derived secret in the new format", func() {
			Eventually(func(g Gomega) {
				migratedSecret := getMigratedSecret()
				g.Expect(migratedSecret.Type).To(Equal(corev1.SecretTypeOpaque))
				g.Expect(migratedSecret.Data).To(MatchAllKeys(Keys{
					korifiv1alpha1.CredentialsSecretKey: Not(BeEmpty()),
				}))

				credentials := map[string]any{}
				g.Expect(json.Unmarshal(migratedSecret.Data[korifiv1alpha1.CredentialsSecretKey], &credentials)).To(Succeed())
				g.Expect(credentials).To(MatchAllKeys(Keys{
					"foo": Equal("bar"),
				}))
			}).Should(Succeed())
		})

		It("sets an owner reference from the service instance to the migrated secret", func() {
			Eventually(func(g Gomega) {
				migratedSecret := getMigratedSecret()
				g.Expect(migratedSecret.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Kind": Equal("CFServiceInstance"),
					"Name": Equal(instance.Name),
				})))
			}).Should(Succeed())
		})

		It("sets the instance credentials secret name and observed version to the migrated secret name and version", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(instance.Status.Credentials.Name).To(Equal(instance.Name + "-migrated"))
				g.Expect(instance.Status.CredentialsObservedVersion).To(Equal(getMigratedSecret().ResourceVersion))
			}).Should(Succeed())
		})

		It("does not change the original credentials secret", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
				g.Expect(instance.Status.Credentials.Name).NotTo(BeEmpty())

				g.Expect(instance.Spec.SecretName).To(Equal(credentialsSecret.Name))

				previousCredentialsVersion := credentialsSecret.ResourceVersion
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)).To(Succeed())
				g.Expect(credentialsSecret.ResourceVersion).To(Equal(previousCredentialsVersion))
			}).Should(Succeed())
		})

		When("legacy secret cannot be migrated", func() {
			BeforeEach(func() {
				Expect(adminClient.Create(ctx, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      instance.Name + "-migrated",
						Namespace: instance.Namespace,
					},
					Type: corev1.SecretType("will-clash-with-migrated-secret-type"),
				})).To(Succeed())
			})

			It("sets the CredentialSecretAvailable condition to false", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)).To(Succeed())
					g.Expect(instance.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasReason(Equal("FailedReconcilingCredentialsSecret")),
					)))
				}).Should(Succeed())
			})
		})
	})
})
