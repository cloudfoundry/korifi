package handlers_test

import (
	"net/http"

	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ServiceOffering", func() {
	var serviceOfferingRepo *fake.CFServiceOfferingRepository

	BeforeEach(func() {
		serviceOfferingRepo = new(fake.CFServiceOfferingRepository)

		apiHandler := NewServiceOffering(
			*serverURL,
			serviceOfferingRepo,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	Describe("GET /v3/service_offerings", func() {
		BeforeEach(func() {
			serviceOfferingRepo.ListOfferingsReturns([]repositories.ServiceOfferingResource{{
				ServiceOffering: services.ServiceOffering{},
				CFResource: model.CFResource{
					GUID: "offering-guid",
				},
				Relationships: repositories.ServiceOfferingRelationships{
					ServiceBroker: model.ToOneRelationship{
						Data: model.Relationship{
							GUID: "broker-guid",
						},
					},
				},
			}}, nil)
		})

		JustBeforeEach(func() {
			req, err := http.NewRequestWithContext(ctx, "GET", "/v3/service_offerings", nil)
			Expect(err).NotTo(HaveOccurred())

			routerBuilder.Build().ServeHTTP(rr, req)
		})

		It("lists the service offerings", func() {
			Expect(serviceOfferingRepo.ListOfferingsCallCount()).To(Equal(1))
			_, actualAuthInfo := serviceOfferingRepo.ListOfferingsArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))

			Expect(rr).Should(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.total_results", BeEquivalentTo(1)),
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/service_offerings"),
				MatchJSONPath("$.resources[0].guid", "offering-guid"),
				MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/service_offerings/offering-guid"),
				MatchJSONPath("$.resources[0].links.service_plans.href", "https://api.example.org/v3/service_plans?service_offering_guids=offering-guid"),
				MatchJSONPath("$.resources[0].links.service_broker.href", "https://api.example.org/v3/service_brokers/broker-guid"),
			)))
		})
	})
})
