package brokers_test

import (
	"errors"
	"slices"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/google/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/brokers/fake"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/osbapi"
	"code.cloudfoundry.org/korifi/model/services"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("CFServiceBroker", func() {
	var (
		brokerClient  *fake.BrokerClient
		brokerSecret  *corev1.Secret
		serviceBroker *korifiv1alpha1.CFServiceBroker
	)

	BeforeEach(func() {
		brokerClient = new(fake.BrokerClient)
		brokerClientFactory.CreateClientReturns(brokerClient, nil)
		brokerClient.GetCatalogReturns(osbapi.Catalog{
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
				Plans: []osbapi.Plan{{
					ID:          "plan-id",
					Name:        "plan-name",
					Description: "plan description",
					Metadata: map[string]any{
						"plan-md": "plan-md-value",
					},
					Free:             true,
					Bindable:         true,
					BindingRotatable: true,
					PlanUpdateable:   true,
					Schemas: services.ServicePlanSchemas{
						ServiceInstance: services.ServiceInstanceSchema{
							Create: services.InputParameterSchema{
								Parameters: &runtime.RawExtension{
									Raw: []byte(`{"create-param":"create-value"}`),
								},
							},
							Update: services.InputParameterSchema{
								Parameters: &runtime.RawExtension{
									Raw: []byte(`{"update-param":"update-value"}`),
								},
							},
						},
						ServiceBinding: services.ServiceBindingSchema{
							Create: services.InputParameterSchema{
								Parameters: &runtime.RawExtension{
									Raw: []byte(`{"binding-create-param":"binding-create-value"}`),
								},
							},
						},
					},
				}},
			}},
		}, nil)

		brokerSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rootNamespace,
				Name:      uuid.NewString(),
			},
		}
		Expect(adminClient.Create(ctx, brokerSecret)).To(Succeed())

		serviceBroker = &korifiv1alpha1.CFServiceBroker{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: rootNamespace,
				Name:      uuid.NewString(),
			},
			Spec: korifiv1alpha1.CFServiceBrokerSpec{
				ServiceBroker: services.ServiceBroker{
					Name: "my-service-broker",
					URL:  "some-url",
				},
				Credentials: corev1.LocalObjectReference{
					Name: brokerSecret.Name,
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
			g.Expect(adminClient.List(ctx, offerings,
				client.InNamespace(serviceBroker.Namespace),
				client.MatchingLabels{korifiv1alpha1.RelServiceBrokerGUIDLabel: serviceBroker.Name},
			)).To(Succeed())
			g.Expect(offerings.Items).To(HaveLen(1))

			offering := offerings.Items[0]
			g.Expect(offering.Labels).To(SatisfyAll(
				HaveKeyWithValue(korifiv1alpha1.RelServiceBrokerGUIDLabel, serviceBroker.Name),
				HaveKeyWithValue(korifiv1alpha1.RelServiceBrokerNameLabel, serviceBroker.Spec.Name),
			))
			g.Expect(offering.Spec).To(MatchAllFields(Fields{
				"ServiceOffering": MatchAllFields(Fields{
					"Name":             Equal("service-name"),
					"Description":      Equal("service description"),
					"Tags":             ConsistOf("t1"),
					"Requires":         ConsistOf("r1"),
					"DocumentationURL": PointTo(Equal("https://doc.url")),
					"BrokerCatalog": MatchAllFields(Fields{
						"ID": Equal("service-id"),
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

	It("creates CFServicePlans to reflect catalog plans", func() {
		Eventually(func(g Gomega) {
			offerings := &korifiv1alpha1.CFServiceOfferingList{}
			g.Expect(adminClient.List(ctx, offerings,
				client.InNamespace(serviceBroker.Namespace),
				client.MatchingLabels{korifiv1alpha1.RelServiceBrokerGUIDLabel: serviceBroker.Name},
			)).To(Succeed())
			g.Expect(offerings.Items).To(HaveLen(1))

			plans := &korifiv1alpha1.CFServicePlanList{}
			g.Expect(adminClient.List(ctx, plans,
				client.InNamespace(serviceBroker.Namespace),
				client.MatchingLabels{korifiv1alpha1.RelServiceBrokerGUIDLabel: serviceBroker.Name},
			)).To(Succeed())
			g.Expect(plans.Items).To(HaveLen(1))

			plan := plans.Items[0]

			g.Expect(plan.Labels).To(SatisfyAll(
				HaveKeyWithValue(korifiv1alpha1.RelServiceBrokerGUIDLabel, serviceBroker.Name),
				HaveKeyWithValue(korifiv1alpha1.RelServiceBrokerNameLabel, "my-service-broker"),
				HaveKeyWithValue(korifiv1alpha1.RelServiceOfferingGUIDLabel, offerings.Items[0].Name),
				HaveKeyWithValue(korifiv1alpha1.RelServiceOfferingNameLabel, "service-name"),
			))
			g.Expect(plan.Spec).To(MatchAllFields(Fields{
				"ServicePlan": MatchAllFields(Fields{
					"Name":        Equal("plan-name"),
					"Free":        BeTrue(),
					"Description": Equal("plan description"),
					"BrokerCatalog": MatchAllFields(Fields{
						"ID": Equal("plan-id"),
						"Metadata": PointTo(MatchFields(IgnoreExtras, Fields{
							"Raw": MatchJSON(`{"plan-md": "plan-md-value"}`),
						})),
						"Features": Equal(services.ServicePlanFeatures{
							PlanUpdateable: true,
							Bindable:       true,
						}),
					}),
					"Schemas": MatchFields(IgnoreExtras, Fields{
						"ServiceInstance": MatchFields(IgnoreExtras, Fields{
							"Create": MatchFields(IgnoreExtras, Fields{
								"Parameters": PointTo(MatchFields(IgnoreExtras, Fields{
									"Raw": MatchJSON(`{"create-param":"create-value"}`),
								})),
							}),
							"Update": MatchFields(IgnoreExtras, Fields{
								"Parameters": PointTo(MatchFields(IgnoreExtras, Fields{
									"Raw": MatchJSON(`{"update-param":"update-value"}`),
								})),
							}),
						}),
						"ServiceBinding": MatchFields(IgnoreExtras, Fields{
							"Create": MatchFields(IgnoreExtras, Fields{
								"Parameters": PointTo(MatchFields(IgnoreExtras, Fields{
									"Raw": MatchJSON(`{"binding-create-param": "binding-create-value"}`),
								})),
							}),
						}),
					}),
				}),
				"Visibility": MatchAllFields(Fields{
					"Type":          Equal(korifiv1alpha1.AdminServicePlanVisibilityType),
					"Organizations": BeEmpty(),
				}),
			}))
		}).Should(Succeed())
	})

	When("the plan visibility is updated", func() {
		var planGUID string

		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
				g.Expect(serviceBroker.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionTrue)),
				)))
			}).Should(Succeed())

			plans := &korifiv1alpha1.CFServicePlanList{}
			Expect(adminClient.List(ctx, plans,
				client.InNamespace(serviceBroker.Namespace),
				client.MatchingLabels{korifiv1alpha1.RelServiceBrokerGUIDLabel: serviceBroker.Name},
			)).To(Succeed())
			Expect(plans.Items).To(HaveLen(1))

			plan := &plans.Items[0]
			planGUID = plan.Name

			Expect(k8s.PatchResource(ctx, adminClient, plan, func() {
				plan.Spec.Visibility.Type = korifiv1alpha1.PublicServicePlanVisibilityType
			})).To(Succeed())

			Expect(k8s.PatchResource(ctx, adminClient, serviceBroker, func() {
				serviceBroker.Spec.Name = uuid.NewString()
			})).To(Succeed())
		})

		It("keeps the updated value", func() {
			plan := &korifiv1alpha1.CFServicePlan{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: serviceBroker.Namespace,
					Name:      planGUID,
				},
			}

			Consistently(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(plan), plan)).To(Succeed())
				g.Expect(plan.Spec.Visibility.Type).To(Equal(korifiv1alpha1.PublicServicePlanVisibilityType))
			}).Should(Succeed())
		})
	})

	When("there are multiple brokers serving the same catalog", func() {
		var anotherServiceBroker *korifiv1alpha1.CFServiceBroker

		BeforeEach(func() {
			anotherServiceBroker = &korifiv1alpha1.CFServiceBroker{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceBrokerSpec{
					Credentials: corev1.LocalObjectReference{
						Name: brokerSecret.Name,
					},
				},
			}
			Expect(adminClient.Create(ctx, anotherServiceBroker)).To(Succeed())
		})

		It("creates an offering per broker", func() {
			Eventually(func(g Gomega) {
				offerings := &korifiv1alpha1.CFServiceOfferingList{}
				g.Expect(adminClient.List(ctx, offerings, client.InNamespace(rootNamespace))).To(Succeed())

				brokerOfferings := slices.Collect(it.Filter(itx.FromSlice(offerings.Items), func(o korifiv1alpha1.CFServiceOffering) bool {
					return o.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel] == serviceBroker.Name
				}))
				g.Expect(brokerOfferings).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"ServiceOffering": MatchFields(IgnoreExtras, Fields{
								"BrokerCatalog": MatchFields(IgnoreExtras, Fields{
									"ID": Equal("service-id"),
								}),
							}),
						}),
					}),
				))

				anotherBrokerOfferings := slices.Collect(it.Filter(itx.FromSlice(offerings.Items), func(o korifiv1alpha1.CFServiceOffering) bool {
					return o.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel] == anotherServiceBroker.Name
				}))
				g.Expect(anotherBrokerOfferings).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"ServiceOffering": MatchFields(IgnoreExtras, Fields{
								"BrokerCatalog": MatchFields(IgnoreExtras, Fields{
									"ID": Equal("service-id"),
								}),
							}),
						}),
					}),
				))
			}).Should(Succeed())
		})

		It("creates a plan per broker", func() {
			Eventually(func(g Gomega) {
				plans := &korifiv1alpha1.CFServicePlanList{}
				g.Expect(adminClient.List(ctx, plans, client.InNamespace(serviceBroker.Namespace))).To(Succeed())

				brokerPlans := slices.Collect(it.Filter(itx.FromSlice(plans.Items), func(o korifiv1alpha1.CFServicePlan) bool {
					return o.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel] == serviceBroker.Name
				}))
				g.Expect(brokerPlans).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"ServicePlan": MatchFields(IgnoreExtras, Fields{
								"BrokerCatalog": MatchFields(IgnoreExtras, Fields{
									"ID": Equal("plan-id"),
								}),
							}),
						}),
					}),
				))

				anotherBrokerPlans := slices.Collect(it.Filter(itx.FromSlice(plans.Items), func(o korifiv1alpha1.CFServicePlan) bool {
					return o.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel] == anotherServiceBroker.Name
				}))
				g.Expect(anotherBrokerPlans).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"ServicePlan": MatchFields(IgnoreExtras, Fields{
								"BrokerCatalog": MatchFields(IgnoreExtras, Fields{
									"ID": Equal("plan-id"),
								}),
							}),
						}),
					}),
				))
			}).Should(Succeed())
		})
	})

	It("sets the credentials secret observed version", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
			g.Expect(serviceBroker.Status.CredentialsObservedVersion).NotTo(BeEmpty())
		}).Should(Succeed())
	})

	When("the credentials secret does not exist", func() {
		BeforeEach(func() {
			Expect(adminClient.Delete(ctx, brokerSecret)).To(Succeed())
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

	When("the credentials secret is updated", func() {
		var credentialsObservedVersion string

		JustBeforeEach(func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(serviceBroker.Status.Conditions, korifiv1alpha1.StatusConditionReady)).To(BeTrue())
			}).Should(Succeed())
			credentialsObservedVersion = serviceBroker.Status.CredentialsObservedVersion

			Expect(k8s.Patch(ctx, adminClient, brokerSecret, func() {
				brokerSecret.StringData = map[string]string{"f": "b"}
			})).To(Succeed())
		})

		It("triggers the reconciler", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
				g.Expect(serviceBroker.Status.CredentialsObservedVersion).NotTo(Equal(credentialsObservedVersion))
			}).Should(Succeed())
		})
	})

	When("osbapi client creation fails", func() {
		BeforeEach(func() {
			brokerClientFactory.CreateClientReturns(nil, errors.New("factory-err"))
		})

		It("sets the ready condition to false", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(serviceBroker), serviceBroker)).To(Succeed())
				g.Expect(serviceBroker.Status.Conditions).To(ContainElement(SatisfyAll(
					HasType(Equal(korifiv1alpha1.StatusConditionReady)),
					HasStatus(Equal(metav1.ConditionFalse)),
					HasReason(Equal("OSBAPIClientCreationFailed")),
				)))
			}).Should(Succeed())
		})
	})

	When("getting the catalog fails", func() {
		BeforeEach(func() {
			brokerClient.GetCatalogReturns(osbapi.Catalog{}, errors.New("get-catalog-err"))
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
})
