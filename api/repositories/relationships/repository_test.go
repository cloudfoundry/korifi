package relationships_test

import (
	"errors"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/relationships"
	"code.cloudfoundry.org/korifi/api/repositories/relationships/fake"
	"code.cloudfoundry.org/korifi/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ResourceRelationshipsRepository", func() {
	var (
		serviceOfferingRepo *fake.ServiceOfferingRepository
		serviceBrokerRepo   *fake.ServiceBrokerRepository
		servicePlanRepo     *fake.ServicePlanRepository
		relationshipsRepo   relationships.ResourceRelationshipsRepo

		resourceType   string
		inputResource  *fake.Resource
		inputResources []relationships.Resource
		result         []relationships.Resource
		listError      error
	)

	BeforeEach(func() {
		resourceType = "foo"
		inputResource = new(fake.Resource)
		inputResources = []relationships.Resource{inputResource}
		inputResource.RelationshipsReturns(map[string]string{
			"foo": "foo-guid",
		})

		serviceOfferingRepo = new(fake.ServiceOfferingRepository)
		serviceBrokerRepo = new(fake.ServiceBrokerRepository)
		servicePlanRepo = new(fake.ServicePlanRepository)
		relationshipsRepo = *relationships.NewResourseRelationshipsRepo(serviceOfferingRepo, serviceBrokerRepo, servicePlanRepo)
	})

	JustBeforeEach(func() {
		result, listError = relationshipsRepo.ListRelatedResources(ctx, authInfo, resourceType, inputResources)
	})

	It("errors", func() {
		Expect(listError).To(MatchError(ContainSubstring(`no repository for type "foo"`)))
	})

	When("the resource has no relationships", func() {
		BeforeEach(func() {
			resourceType = "service_offering"
			inputResource.RelationshipsReturns(nil)
		})

		It("returns an empty list", func() {
			Expect(listError).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})

		It("does not invoke the delegate repository", func() {
			Expect(listError).NotTo(HaveOccurred())
			Expect(serviceOfferingRepo.ListOfferingsCallCount()).To(BeZero())
		})
	})

	Describe("resource type service_offering", func() {
		BeforeEach(func() {
			resourceType = "service_offering"

			inputResource.RelationshipsReturns(map[string]string{
				"service_offering": "service-offering-guid",
			})

			serviceOfferingRepo.ListOfferingsReturns([]repositories.ServiceOfferingRecord{{
				CFResource: model.CFResource{
					GUID: "service-offering-guid",
				},
			}}, nil)
		})

		It("delegates to the service_offering repository", func() {
			Expect(serviceOfferingRepo.ListOfferingsCallCount()).To(Equal(1))
			_, _, acutalMessage := serviceOfferingRepo.ListOfferingsArgsForCall(0)
			Expect(acutalMessage).To(Equal(repositories.ListServiceOfferingMessage{
				GUIDs: []string{"service-offering-guid"},
			}))
		})

		It("returns a list of related service offering", func() {
			Expect(listError).NotTo(HaveOccurred())
			Expect(result).To(ConsistOf(
				repositories.ServiceOfferingRecord{
					CFResource: model.CFResource{
						GUID: "service-offering-guid",
					},
				},
			))
		})

		When("the underlying repo returns an error", func() {
			BeforeEach(func() {
				serviceOfferingRepo.ListOfferingsReturns(nil, errors.New("list-offering-error"))
			})

			It("returns an error", func() {
				Expect(listError).To(MatchError("list-offering-error"))
			})
		})
	})

	Describe("resource type service_broker", func() {
		BeforeEach(func() {
			resourceType = "service_broker"

			inputResource.RelationshipsReturns(map[string]string{
				"service_broker": "service-broker-guid",
			})

			serviceBrokerRepo.ListServiceBrokersReturns([]repositories.ServiceBrokerRecord{{
				CFResource: model.CFResource{
					GUID: "service-broker-guid",
				},
			}}, nil)
		})

		It("delegates to the service_broker repository", func() {
			Expect(serviceBrokerRepo.ListServiceBrokersCallCount()).To(Equal(1))
			_, _, acutalMessage := serviceBrokerRepo.ListServiceBrokersArgsForCall(0)
			Expect(acutalMessage).To(Equal(repositories.ListServiceBrokerMessage{
				GUIDs: []string{"service-broker-guid"},
			}))
		})

		It("returns a list of related service broker", func() {
			Expect(listError).NotTo(HaveOccurred())
			Expect(result).To(ConsistOf(
				repositories.ServiceBrokerRecord{
					CFResource: model.CFResource{
						GUID: "service-broker-guid",
					},
				},
			))
		})

		When("the underlying repo returns an error", func() {
			BeforeEach(func() {
				serviceBrokerRepo.ListServiceBrokersReturns(nil, errors.New("list-broker-error"))
			})

			It("returns an error", func() {
				Expect(listError).To(MatchError("list-broker-error"))
			})
		})
	})

	Describe("resource type service_plan", func() {
		BeforeEach(func() {
			resourceType = "service_plan"

			inputResource.RelationshipsReturns(map[string]string{
				"service_plan": "service-plan-guid",
			})

			servicePlanRepo.ListPlansReturns([]repositories.ServicePlanRecord{{
				CFResource: model.CFResource{
					GUID: "service-plan-guid",
				},
			}}, nil)
		})

		It("delegates to the service_plan repository", func() {
			Expect(servicePlanRepo.ListPlansCallCount()).To(Equal(1))
			_, _, acutalMessage := servicePlanRepo.ListPlansArgsForCall(0)
			Expect(acutalMessage).To(Equal(repositories.ListServicePlanMessage{
				GUIDs: []string{"service-plan-guid"},
			}))
		})

		It("returns a list of related service plan", func() {
			Expect(listError).NotTo(HaveOccurred())
			Expect(result).To(ConsistOf(
				repositories.ServicePlanRecord{
					CFResource: model.CFResource{
						GUID: "service-plan-guid",
					},
				},
			))
		})

		When("the underlying repo returns an error", func() {
			BeforeEach(func() {
				servicePlanRepo.ListPlansReturns(nil, errors.New("list-plan-error"))
			})

			It("returns an error", func() {
				Expect(listError).To(MatchError("list-plan-error"))
			})
		})
	})

	Describe("resource type space", func() {
		BeforeEach(func() {
			resourceType = "space"
		})

		It("returns a empty list", func() {
			Expect(listError).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})

	Describe("resource type space", func() {
		BeforeEach(func() {
			resourceType = "space"
		})

		It("returns a empty list", func() {
			Expect(listError).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})
})
