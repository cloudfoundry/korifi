package repositories_test

import (
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServicePlanRepo", func() {
	var repo *repositories.ServicePlanRepo

	BeforeEach(func() {
		repo = repositories.NewServicePlanRepo(userClientFactory, rootNamespace)
	})

	Describe("List", func() {
		var (
			planGUID    string
			listedPlans []repositories.ServicePlanRecord
			message     repositories.ListServicePlanMessage
			listErr     error
		)

		BeforeEach(func() {
			planGUID = uuid.NewString()
			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServicePlan{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      planGUID,
					Labels: map[string]string{
						korifiv1alpha1.RelServiceOfferingLabel: "offering-guid",
					},
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
				},
				Spec: korifiv1alpha1.CFServicePlanSpec{
					ServicePlan: services.ServicePlan{
						Name:        "my-service-plan",
						Free:        true,
						Description: "service plan description",
						BrokerCatalog: services.ServicePlanBrokerCatalog{
							ID: "broker-plan-guid",
							Metadata: &runtime.RawExtension{
								Raw: []byte(`{"foo":"bar"}`),
							},
							Features: services.ServicePlanFeatures{
								PlanUpdateable: true,
								Bindable:       true,
							},
						},
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
					},
					Visibility: korifiv1alpha1.ServicePlanVisibility{
						Type: korifiv1alpha1.AdminServicePlanVisibilityType,
					},
				},
			})).To(Succeed())

			message = repositories.ListServicePlanMessage{}
		})

		JustBeforeEach(func() {
			listedPlans, listErr = repo.ListPlans(ctx, authInfo, message)
		})

		It("lists service offerings", func() {
			Expect(listErr).NotTo(HaveOccurred())
			Expect(listedPlans).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
				"ServicePlan": MatchFields(IgnoreExtras, Fields{
					"Name":        Equal("my-service-plan"),
					"Description": Equal("service plan description"),
					"Free":        BeTrue(),
					"BrokerCatalog": MatchFields(IgnoreExtras, Fields{
						"ID": Equal("broker-plan-guid"),
						"Metadata": PointTo(MatchFields(IgnoreExtras, Fields{
							"Raw": MatchJSON(`{"foo": "bar"}`),
						})),

						"Features": MatchFields(IgnoreExtras, Fields{
							"PlanUpdateable": BeTrue(),
							"Bindable":       BeTrue(),
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
				"CFResource": MatchFields(IgnoreExtras, Fields{
					"GUID":      Equal(planGUID),
					"CreatedAt": Not(BeZero()),
					"UpdatedAt": BeNil(),
					"Metadata": MatchAllFields(Fields{
						"Labels":      HaveKeyWithValue(korifiv1alpha1.RelServiceOfferingLabel, "offering-guid"),
						"Annotations": HaveKeyWithValue("annotation", "annotation-value"),
					}),
				}),
				"Relationships": Equal(repositories.ServicePlanRelationships{
					ServiceOffering: model.ToOneRelationship{
						Data: model.Relationship{
							GUID: "offering-guid",
						},
					},
				}),
			})))
		})

		When("filtering by service_offering_guid", func() {
			BeforeEach(func() {
				Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServicePlan{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: rootNamespace,
						Name:      uuid.NewString(),
						Labels: map[string]string{
							korifiv1alpha1.RelServiceOfferingLabel: "other-offering-guid",
						},
					},
					Spec: korifiv1alpha1.CFServicePlanSpec{
						Visibility: korifiv1alpha1.ServicePlanVisibility{
							Type: korifiv1alpha1.AdminServicePlanVisibilityType,
						},
					},
				})).To(Succeed())

				message.ServiceOfferingGUIDs = []string{"other-offering-guid"}
			})

			It("returns matching service plans", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(listedPlans).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"Relationships": Equal(repositories.ServicePlanRelationships{
						ServiceOffering: model.ToOneRelationship{
							Data: model.Relationship{
								GUID: "other-offering-guid",
							},
						},
					}),
				})))
			})
		})
	})

	Describe("GetPlanVisibility", func() {
		var (
			planGUID   string
			visibility repositories.ServicePlanVisibilityRecord
		)

		BeforeEach(func() {
			planGUID = uuid.NewString()
			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServicePlan{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      planGUID,
				},
				Spec: korifiv1alpha1.CFServicePlanSpec{
					Visibility: korifiv1alpha1.ServicePlanVisibility{
						Type: korifiv1alpha1.AdminServicePlanVisibilityType,
					},
				},
			})).To(Succeed())
		})

		JustBeforeEach(func() {
			var err error
			visibility, err = repo.GetPlanVisibility(ctx, authInfo, planGUID)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the plan visibility", func() {
			Expect(visibility).To(Equal(repositories.ServicePlanVisibilityRecord{
				Type: korifiv1alpha1.AdminServicePlanVisibilityType,
			}))
		})
	})
})
