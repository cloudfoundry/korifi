package repositories_test

import (
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/model"
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
		repo = repositories.NewServiceOfferingRepo(userClientFactory, rootNamespace)
	})

	Describe("List", func() {
		var (
			offeringGUID    string
			listedOfferings []repositories.ServiceOfferingResource
			listErr         error
		)

		BeforeEach(func() {
			offeringGUID = uuid.NewString()
			Expect(k8sClient.Create(ctx, &korifiv1alpha1.CFServiceOffering{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: rootNamespace,
					Name:      offeringGUID,
					Labels: map[string]string{
						korifiv1alpha1.RelServiceBrokerLabel: "broker-guid",
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
							Id: "offering-catalog-guid",
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
		})

		JustBeforeEach(func() {
			listedOfferings, listErr = repo.ListOfferings(ctx, authInfo)
		})

		It("lists service offerings", func() {
			Expect(listErr).NotTo(HaveOccurred())
			Expect(listedOfferings).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
				"ServiceOffering": MatchFields(IgnoreExtras, Fields{
					"Name":             Equal("my-offering"),
					"Description":      Equal("my offering description"),
					"Tags":             ConsistOf("t1"),
					"Requires":         ConsistOf("r1"),
					"DocumentationURL": PointTo(Equal("https://my.offering.com")),
					"BrokerCatalog": MatchFields(IgnoreExtras, Fields{
						"Id": Equal("offering-catalog-guid"),
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
				}),
				"Relationships": Equal(repositories.ServiceOfferingRelationships{
					ServiceBroker: model.ToOneRelationship{
						Data: model.Relationship{
							GUID: "broker-guid",
						},
					},
				}),
			})))
		})
	})
})
