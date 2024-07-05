package brokers_test

import (
	"github.com/google/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/brokers/osbapi"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tests/helpers/broker"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFServiceBroker", func() {
	var (
		testNamespace     string
		serviceBroker     *korifiv1alpha1.CFServiceBroker
		credentialsSecret *corev1.Secret
	)

	BeforeEach(func() {
		testNamespace = uuid.NewString()
		Expect(adminClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		})).To(Succeed())

		brokerServer := broker.NewServer().WithCatalog(&osbapi.Catalog{
			Services: []osbapi.Service{{
				ID:          "service-id",
				Name:        "service-name",
				Description: "service description",
				BrokerCatalogFeatures: services.BrokerCatalogFeatures{
					Bindable:             true,
					InstancesRetrievable: true,
					BindingsRetrievable:  true,
					PlanUpdateable:       true,
					AllowContextUpdates:  true,
				},
				Tags:     []string{"t1"},
				Requires: []string{"r1"},
				Metadata: map[string]any{
					"foo":              "bar",
					"documentationUrl": "https://doc.url",
				},
			}},
		}).Start()

		DeferCleanup(func() {
			brokerServer.Stop()
		})

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

		serviceBroker = &korifiv1alpha1.CFServiceBroker{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: testNamespace,
				Name:      uuid.NewString(),
			},
			Spec: korifiv1alpha1.CFServiceBrokerSpec{
				ServiceBroker: services.ServiceBroker{
					URL: brokerServer.URL(),
				},
				Credentials: corev1.LocalObjectReference{
					Name: credentialsSecret.Name,
				},
			},
		}
		Expect(adminClient.Create(ctx, serviceBroker)).To(Succeed())
	})

	It("sets the Ready condition to true", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
			g.Expect(serviceBroker.Status.Conditions).To(ContainElement(SatisfyAll(
				HasType(Equal(korifiv1alpha1.StatusConditionReady)),
				HasStatus(Equal(metav1.ConditionTrue)),
			)))
		}).Should(Succeed())
	})
	It("sets the ObservedGeneration status field", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
			g.Expect(serviceBroker.Status.ObservedGeneration).To(Equal(serviceBroker.Generation))
		}).Should(Succeed())
	})

	It("creates CFServiceOfferings to reflect the catalog offerings", func() {
		Eventually(func(g Gomega) {
			offerings := &korifiv1alpha1.CFServiceOfferingList{}
			g.Expect(adminClient.List(ctx, offerings, client.InNamespace(serviceBroker.Namespace))).To(Succeed())
			g.Expect(offerings.Items).To(HaveLen(1))

			offering := offerings.Items[0]
			g.Expect(offering.Labels).To(HaveKeyWithValue(korifiv1alpha1.RelServiceBrokerLabel, serviceBroker.Name))
			g.Expect(offering.Spec).To(MatchAllFields(Fields{
				"ServiceOffering": MatchAllFields(Fields{
					"Name":             Equal("service-name"),
					"Description":      Equal("service description"),
					"Tags":             ConsistOf("t1"),
					"Requires":         ConsistOf("r1"),
					"DocumentationURL": PointTo(Equal("https://doc.url")),
					"BrokerCatalog": MatchAllFields(Fields{
						"Id": Equal("service-id"),
						"Features": MatchAllFields(Fields{
							"PlanUpdateable":       BeTrue(),
							"Bindable":             BeTrue(),
							"InstancesRetrievable": BeTrue(),
							"BindingsRetrievable":  BeTrue(),
							"AllowContextUpdates":  BeTrue(),
						}),
						"Metadata": PointTo(MatchFields(IgnoreExtras, Fields{
							"Raw": MatchJSON(`{
								"documentationUrl": "https://doc.url",
								"foo": "bar"
							}`),
						})),
					}),
				}),
			}))
		}).Should(Succeed())
	})

	When("getting the catalog fails", func() {
		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, serviceBroker, func() {
				serviceBroker.Spec.URL = "https://must.not.exist"
			})).To(Succeed())
		})

		It("sets the ready condition to False", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
				g.Expect(serviceBroker.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
					HasReason(Equal("GetCatalogFailed")),
				)))
			}).Should(Succeed())
		})
	})

	When("there are multiple brokers serving the same catalog", func() {
		var anotherServiceBroker *korifiv1alpha1.CFServiceBroker

		BeforeEach(func() {
			anotherServiceBroker = &korifiv1alpha1.CFServiceBroker{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceBrokerSpec{
					ServiceBroker: services.ServiceBroker{
						URL: serviceBroker.Spec.URL,
					},
					Credentials: corev1.LocalObjectReference{
						Name: credentialsSecret.Name,
					},
				},
			}
			Expect(adminClient.Create(ctx, anotherServiceBroker)).To(Succeed())
		})

		It("creates an offering per broker", func() {
			Eventually(func(g Gomega) {
				offerings := &korifiv1alpha1.CFServiceOfferingList{}
				g.Expect(adminClient.List(ctx, offerings, client.InNamespace(testNamespace))).To(Succeed())
				g.Expect(offerings.Items).To(HaveLen(2))

				brokerGUIDs := []string{
					offerings.Items[0].Labels[korifiv1alpha1.RelServiceBrokerLabel],
					offerings.Items[1].Labels[korifiv1alpha1.RelServiceBrokerLabel],
				}
				g.Expect(brokerGUIDs).To(ConsistOf(serviceBroker.Name, anotherServiceBroker.Name))

				g.Expect(offerings.Items[0].Spec.BrokerCatalog.Id).To(Equal("service-id"))
				g.Expect(offerings.Items[1].Spec.BrokerCatalog.Id).To(Equal("service-id"))
			}).Should(Succeed())
		})
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
			Expect(k8s.PatchResource(ctx, adminClient, serviceBroker, func() {
				serviceBroker.Spec.Credentials.Name = credentialsSecret.Name
			})).To(Succeed())
		})

		It("sets the Ready condition to true", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
				g.Expect(serviceBroker.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionTrue)),
				)))
			}).Should(Succeed())
		})

		It("sets the credentials secret observed version", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
				g.Expect(serviceBroker.Status.CredentialsObservedVersion).NotTo(BeEmpty())
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
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
					g.Expect(serviceBroker.Status.Conditions).To(ContainElement(SatisfyAll(
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
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
					g.Expect(serviceBroker.Status.Conditions).To(ContainElement(SatisfyAll(
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
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
					g.Expect(serviceBroker.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasReason(Equal("SecretInvalid")),
					)))
				}).Should(Succeed())
			})
		})

		When("the credentials secret does not exist", func() {
			BeforeEach(func() {
				Expect(adminClient.Delete(ctx, credentialsSecret)).To(Succeed())
			})

			It("sets the Ready condition to false", func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
					g.Expect(serviceBroker.Status.Conditions).To(ContainElement(SatisfyAll(
						HasType(Equal(korifiv1alpha1.StatusConditionReady)),
						HasStatus(Equal(metav1.ConditionFalse)),
						HasReason(Equal("CredentialsSecretNotAvailable")),
					)))
				}).Should(Succeed())
			})
		})

		When("the credentials secret is reconciled", func() {
			var credentialsObservedVersion string

			BeforeEach(func() {
				Eventually(func(g Gomega) {
					g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
					g.Expect(meta.IsStatusConditionTrue(serviceBroker.Status.Conditions, korifiv1alpha1.StatusConditionReady)).To(BeTrue())
				}).Should(Succeed())
				credentialsObservedVersion = serviceBroker.Status.CredentialsObservedVersion
			})

			When("the credentials secret changes", func() {
				BeforeEach(func() {
					Expect(k8s.Patch(ctx, adminClient, credentialsSecret, func() {
						credentialsSecret.StringData = map[string]string{"f": "b"}
					})).To(Succeed())
				})

				It("updates the credentials secret observed version", func() {
					Eventually(func(g Gomega) {
						g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
						g.Expect(serviceBroker.Status.CredentialsObservedVersion).NotTo(Equal(credentialsObservedVersion))
					}).Should(Succeed())
				})
			})
		})
	})
})
