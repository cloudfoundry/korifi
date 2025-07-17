package relationships_test

import (
	"errors"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	"code.cloudfoundry.org/korifi/api/repositories/relationships"
	"code.cloudfoundry.org/korifi/api/repositories/relationships/fake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ResourceRelationshipsRepository", func() {
	var (
		serviceOfferingRepo *fake.ServiceOfferingRepository
		serviceBrokerRepo   *fake.ServiceBrokerRepository
		servicePlanRepo     *fake.ServicePlanRepository
		spaceRepo           *fake.SpaceRepository
		orgRepo             *fake.OrgRepository
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
		spaceRepo = new(fake.SpaceRepository)
		orgRepo = new(fake.OrgRepository)
		relationshipsRepo = *relationships.NewResourseRelationshipsRepo(serviceOfferingRepo, serviceBrokerRepo, servicePlanRepo, spaceRepo, orgRepo)
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

			serviceOfferingRepo.ListOfferingsReturns(repositories.ListResult[repositories.ServiceOfferingRecord]{
				Records: []repositories.ServiceOfferingRecord{
					{
						GUID: "service-offering-guid",
					},
				},
				PageInfo: descriptors.PageInfo{
					TotalResults: 1,
					TotalPages:   1,
					PageNumber:   1,
					PageSize:     1,
				},
			}, nil)
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
				repositories.ServiceOfferingRecord{GUID: "service-offering-guid"},
			))
		})

		When("the underlying repo returns an error", func() {
			BeforeEach(func() {
				serviceOfferingRepo.ListOfferingsReturns(repositories.ListResult[repositories.ServiceOfferingRecord]{}, errors.New("list-offering-error"))
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

			serviceBrokerRepo.ListServiceBrokersReturns(repositories.ListResult[repositories.ServiceBrokerRecord]{
				Records: []repositories.ServiceBrokerRecord{{GUID: "service-broker-guid"}},
			}, nil)
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
				repositories.ServiceBrokerRecord{GUID: "service-broker-guid"},
			))
		})

		When("the underlying repo returns an error", func() {
			BeforeEach(func() {
				serviceBrokerRepo.ListServiceBrokersReturns(repositories.ListResult[repositories.ServiceBrokerRecord]{}, errors.New("list-broker-error"))
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

			servicePlanRepo.ListPlansReturns([]repositories.ServicePlanRecord{{GUID: "service-plan-guid"}}, nil)
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
			Expect(result).To(ConsistOf(repositories.ServicePlanRecord{GUID: "service-plan-guid"}))
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

			inputResource.RelationshipsReturns(map[string]string{
				"space": "space-guid",
			})

			spaceRepo.ListSpacesReturns([]repositories.SpaceRecord{{GUID: "space-guid"}}, nil)
		})

		It("returns a list of related spaces", func() {
			Expect(listError).NotTo(HaveOccurred())
			Expect(result).To(ConsistOf(repositories.SpaceRecord{GUID: "space-guid"}), nil)
		})

		When("the underlying repo returns an error", func() {
			BeforeEach(func() {
				spaceRepo.ListSpacesReturns(nil, errors.New("list-space-error"))
			})

			It("returns an error", func() {
				Expect(listError).To(MatchError("list-space-error"))
			})
		})
	})

	Describe("resource type organization", func() {
		BeforeEach(func() {
			resourceType = "organization"

			inputResource.RelationshipsReturns(map[string]string{
				"organization": "org-guid",
			})

			orgRepo.ListOrgsReturns(repositories.ListResult[repositories.OrgRecord]{
				Records: []repositories.OrgRecord{{GUID: "org-guid"}},
			}, nil)
		})

		It("returns a list of related orgs", func() {
			Expect(listError).NotTo(HaveOccurred())
			Expect(result).To(ConsistOf(repositories.OrgRecord{GUID: "org-guid"}), nil)
		})

		When("the underlying repo returns an error", func() {
			BeforeEach(func() {
				orgRepo.ListOrgsReturns(repositories.ListResult[repositories.OrgRecord]{}, errors.New("list-org-error"))
			})

			It("returns an error", func() {
				Expect(listError).To(MatchError("list-org-error"))
			})
		})
	})
})
