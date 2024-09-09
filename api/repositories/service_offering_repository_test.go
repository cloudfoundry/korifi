package repositories_test

import (
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceOfferingRepo", func() {
	var repo *repositories.ServiceOfferingRepo

	BeforeEach(func() {
		repo = repositories.NewServiceOfferingRepo(
			userClientFactory,
			rootNamespace,
			repositories.NewServiceBrokerRepo(
				userClientFactory,
				rootNamespace,
			),
		)
	})

	Describe("List", func() {
		var (
			offeringGUID        string
			anotherOfferingGUID string
			broker              *korifiv1alpha1.CFServiceBroker
			listedOfferings     []repositories.ServiceOfferingRecord
			message             repositories.ListServiceOfferingMessage
			listErr             error
		)

		BeforeEach(func() {
			offeringGUID = uuid.NewString()
			anotherOfferingGUID = uuid.NewString()

			broker = &korifiv1alpha1.CFServiceBroker{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      uuid.NewString(),
				},
				Spec: korifiv1alpha1.CFServiceBrokerSpec{
					ServiceBroker: services.ServiceBroker{
						Name: uuid.NewString(),
					},
				},
			}
			Expect(k8sClient.Create(ctx, broker)).To(Succeed())

			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServiceOffering{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      offeringGUID,
					Labels: map[string]string{
						korifiv1alpha1.RelServiceBrokerGUIDLabel: broker.Name,
						korifiv1alpha1.RelServiceBrokerNameLabel: broker.Spec.Name,
					},
					Annotations: map[string]string{
						"annotation": "annotation-value",
					},
				},
				Spec: korifiv1alpha1.CFServiceOfferingSpec{
					ServiceOffering: services.ServiceOffering{
						Name:             "my-offering",
						Description:      "my offering description",
						Tags:             []string{"t1"},
						Requires:         []string{"r1"},
						DocumentationURL: tools.PtrTo("https://my.offering.com"),
						BrokerCatalog: services.ServiceBrokerCatalog{
							ID: "offering-catalog-guid",
							Metadata: &runtime.RawExtension{
								Raw: []byte(`{"offering-md": "offering-md-value"}`),
							},
							Features: services.BrokerCatalogFeatures{
								PlanUpdateable:       true,
								Bindable:             true,
								InstancesRetrievable: true,
								BindingsRetrievable:  true,
								AllowContextUpdates:  true,
							},
						},
					},
				},
			})).To(Succeed())

			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServiceOffering{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      anotherOfferingGUID,
					Labels: map[string]string{
						korifiv1alpha1.RelServiceBrokerGUIDLabel: "another-broker",
						korifiv1alpha1.RelServiceBrokerNameLabel: "another-broker-name",
					},
				},
				Spec: korifiv1alpha1.CFServiceOfferingSpec{
					ServiceOffering: services.ServiceOffering{
						Name: "another-offering",
					},
				},
			})).To(Succeed())

			message = repositories.ListServiceOfferingMessage{}
		})

		JustBeforeEach(func() {
			listedOfferings, listErr = repo.ListOfferings(ctx, authInfo, message)
		})

		It("lists service offerings", func() {
			Expect(listErr).NotTo(HaveOccurred())
			Expect(listedOfferings).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{
					"ServiceOffering": MatchFields(IgnoreExtras, Fields{
						"Name":             Equal("my-offering"),
						"Description":      Equal("my offering description"),
						"Tags":             ConsistOf("t1"),
						"Requires":         ConsistOf("r1"),
						"DocumentationURL": PointTo(Equal("https://my.offering.com")),
						"BrokerCatalog": MatchFields(IgnoreExtras, Fields{
							"ID": Equal("offering-catalog-guid"),
							"Metadata": PointTo(MatchFields(IgnoreExtras, Fields{
								"Raw": MatchJSON(`{"offering-md": "offering-md-value"}`),
							})),
							"Features": MatchFields(IgnoreExtras, Fields{
								"PlanUpdateable":       BeTrue(),
								"Bindable":             BeTrue(),
								"InstancesRetrievable": BeTrue(),
								"BindingsRetrievable":  BeTrue(),
								"AllowContextUpdates":  BeTrue(),
							}),
						}),
					}),
					"CFResource": MatchFields(IgnoreExtras, Fields{
						"GUID":      Equal(offeringGUID),
						"CreatedAt": Not(BeZero()),
						"UpdatedAt": BeNil(),
						"Metadata": MatchAllFields(Fields{
							"Labels":      HaveKeyWithValue(korifiv1alpha1.RelServiceBrokerGUIDLabel, broker.Name),
							"Annotations": HaveKeyWithValue("annotation", "annotation-value"),
						}),
					}),
					"ServiceBrokerGUID": Equal(broker.Name),
				}),
				MatchFields(IgnoreExtras, Fields{
					"CFResource": MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(anotherOfferingGUID),
					}),
				}),
			))
		})

		When("filtering by name", func() {
			BeforeEach(func() {
				message.Names = []string{"my-offering"}
			})

			It("returns the matching offerings", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(listedOfferings).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"ServiceOffering": MatchFields(IgnoreExtras, Fields{
						"Name": Equal("my-offering"),
					}),
				})))
			})
		})

		When("filtering by broker name", func() {
			BeforeEach(func() {
				message.BrokerNames = []string{broker.Spec.Name}
			})

			It("returns the matching offerings", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(listedOfferings).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"ServiceBrokerGUID": Equal(broker.Name),
				})))
			})
		})

		When("filtering by guid", func() {
			BeforeEach(func() {
				message.GUIDs = []string{offeringGUID}
			})

			It("returns the matching offerings", func() {
				Expect(listErr).NotTo(HaveOccurred())
				Expect(listedOfferings).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
					"CFResource": MatchFields(IgnoreExtras, Fields{
						"GUID": Equal(offeringGUID),
					}),
				})))
			})
		})
	})
})
