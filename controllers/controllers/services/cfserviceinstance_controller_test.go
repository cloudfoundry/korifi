package services_test

import (
	"context"
	"encoding/json"

	. "github.com/onsi/gomega/gstruct"
	"sigs.k8s.io/controller-runtime/pkg/client"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFServiceInstance", func() {
	var (
		namespace         *corev1.Namespace
		credentialsSecret *corev1.Secret
		cfServiceInstance *korifiv1alpha1.CFServiceInstance
	)

	BeforeEach(func() {
		namespace = BuildNamespaceObject(GenerateGUID())
		Expect(
			adminClient.Create(context.Background(), namespace),
		).To(Succeed())

		credentialsSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "secret-name",
				Namespace: namespace.Name,
			},
			Data: map[string][]byte{
				korifiv1alpha1.CredentialsSecretKey: []byte(`{"foo": "bar"}`),
			},
		}

		cfServiceInstance = &korifiv1alpha1.CFServiceInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "service-instance-guid",
				Namespace: namespace.Name,
			},
			Spec: korifiv1alpha1.CFServiceInstanceSpec{
				DisplayName: "service-instance-name",
				Type:        "user-provided",
				Tags:        []string{},
				SecretName:  credentialsSecret.Name,
			},
		}
	})

	AfterEach(func() {
		Expect(adminClient.Delete(context.Background(), namespace)).To(Succeed())
	})

	JustBeforeEach(func() {
		Expect(adminClient.Create(ctx, credentialsSecret)).To(Succeed())
		Expect(adminClient.Create(context.Background(), cfServiceInstance)).To(Succeed())
	})

	It("sets the CredentialsSecretAvailable condition to true in the CFServiceInstance status", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())

			g.Expect(cfServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Type":               Equal(services.CredentialsSecretAvailableCondition),
				"Status":             Equal(metav1.ConditionTrue),
				"Reason":             Equal("SecretFound"),
				"Message":            Equal(""),
				"ObservedGeneration": Equal(cfServiceInstance.Generation),
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

	When("the credentials secret is invalid", func() {
		BeforeEach(func() {
			credentialsSecret.Data = map[string][]byte{}
		})

		It("sets credentials secret available condition to false", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())

				g.Expect(cfServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
					"Type":               Equal(services.CredentialsSecretAvailableCondition),
					"Status":             Equal(metav1.ConditionFalse),
					"Reason":             Equal("SecretInvalid"),
					"ObservedGeneration": Equal(cfServiceInstance.Generation),
				})))
			}).Should(Succeed())
		})
	})

	When("the credentials secret changes", func() {
		var secretVersion string

		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())
				g.Expect(cfServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
					"Type":    Equal(services.CredentialsSecretAvailableCondition),
					"Status":  Equal(metav1.ConditionTrue),
					"Reason":  Equal("SecretFound"),
					"Message": Equal(""),
				})))
				secretVersion = cfServiceInstance.Status.CredentialsObservedVersion
			}).Should(Succeed())

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

	When("the credentials secret does not exist", func() {
		BeforeEach(func() {
			cfServiceInstance.Spec.SecretName = "other-secret-name"
		})

		It("sets the CredentialsSecretAvailable condition to false in the CFServiceInstance status", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(
					ctx,
					client.ObjectKeyFromObject(cfServiceInstance),
					cfServiceInstance,
				)).To(Succeed())
				g.Expect(meta.IsStatusConditionFalse(
					cfServiceInstance.Status.Conditions,
					services.CredentialsSecretAvailableCondition,
				)).To(BeTrue())
			}).Should(Succeed())
		})
	})

	When("the credentials secret gets deleted", func() {
		var lastObservedVersion string

		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(
					ctx,
					client.ObjectKeyFromObject(cfServiceInstance),
					cfServiceInstance,
				)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(
					cfServiceInstance.Status.Conditions,
					services.CredentialsSecretAvailableCondition,
				)).To(BeTrue())
			}).Should(Succeed())
			lastObservedVersion = cfServiceInstance.Status.CredentialsObservedVersion

			Expect(adminClient.Delete(ctx, credentialsSecret)).To(Succeed())
		})

		It("does not change observed credentials secret", func() {
			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(
					ctx,
					client.ObjectKeyFromObject(cfServiceInstance),
					cfServiceInstance,
				)).To(Succeed())
				g.Expect(cfServiceInstance.Status.Credentials.Name).To(Equal(credentialsSecret.Name))
				g.Expect(cfServiceInstance.Status.CredentialsObservedVersion).To(Equal(lastObservedVersion))
			}).Should(Succeed())
		})
	})

	Describe("legacy credentials secret reconciliation", func() {
		BeforeEach(func() {
			credentialsSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "secret-name",
					Namespace: namespace.Name,
				},
				Type: corev1.SecretType(
					services.ServiceBindingSecretTypePrefix + "legacy",
				),
				StringData: map[string]string{
					"foo": "bar",
				},
			}
		})

		It("migrates the secret", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())
				g.Expect(cfServiceInstance.Status.Credentials.Name).To(Equal(cfServiceInstance.Name + "-migrated"))

				migratedSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfServiceInstance.Name + "-migrated",
						Namespace: namespace.Name,
					},
				}
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(migratedSecret), migratedSecret)).To(Succeed())
				g.Expect(cfServiceInstance.Status.CredentialsObservedVersion).To(Equal(migratedSecret.ResourceVersion))
				g.Expect(migratedSecret.Type).To(Equal(corev1.SecretTypeOpaque))
				g.Expect(migratedSecret.Data).To(MatchAllKeys(Keys{
					korifiv1alpha1.CredentialsSecretKey: Not(BeEmpty()),
				}))
				g.Expect(migratedSecret.OwnerReferences).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Kind": Equal("CFServiceInstance"),
					"Name": Equal(cfServiceInstance.Name),
				})))

				credentials := map[string]any{}
				g.Expect(json.Unmarshal(migratedSecret.Data[korifiv1alpha1.CredentialsSecretKey], &credentials)).To(Succeed())
				g.Expect(credentials).To(MatchAllKeys(Keys{
					"foo": Equal("bar"),
				}))
			}).Should(Succeed())
		})

		It("does not change the original credentials secret", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())
				g.Expect(cfServiceInstance.Status.Credentials.Name).NotTo(BeEmpty())

				g.Expect(cfServiceInstance.Spec.SecretName).To(Equal(credentialsSecret.Name))

				previousCredentialsVersion := credentialsSecret.ResourceVersion
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(credentialsSecret), credentialsSecret)).To(Succeed())
				g.Expect(credentialsSecret.ResourceVersion).To(Equal(previousCredentialsVersion))
			}).Should(Succeed())
		})

		When("legacy secret cannot be migrated", func() {
			BeforeEach(func() {
				Expect(adminClient.Create(ctx, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfServiceInstance.Name + "-migrated",
						Namespace: cfServiceInstance.Namespace,
					},
					Type: corev1.SecretType("legacy"),
				})).To(Succeed())
			})

			It("sets credentials secret not available status condition", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(context.Background(), client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)).To(Succeed())

					g.Expect(cfServiceInstance.Status.Conditions).To(ContainElement(MatchFields(IgnoreExtras, Fields{
						"Type":               Equal(services.CredentialsSecretAvailableCondition),
						"Status":             Equal(metav1.ConditionFalse),
						"Reason":             Equal("FailedReconcilingCredentialsSecret"),
						"ObservedGeneration": Equal(cfServiceInstance.Generation),
					})))
				}).Should(Succeed())
			})
		})
	})
})
