package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/payloads/params"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

var _ = Describe("ServicePlan", func() {
	Describe("List", func() {
		DescribeTable("valid query",
			func(query string, expectedServicePlanList payloads.ServicePlanList) {
				actualServicePlanList, decodeErr := decodeQuery[payloads.ServicePlanList](query)

				Expect(decodeErr).NotTo(HaveOccurred())
				Expect(*actualServicePlanList).To(Equal(expectedServicePlanList))
			},

			Entry("service_offering_guids", "service_offering_guids=b1,b2", payloads.ServicePlanList{ServiceOfferingGUIDs: "b1,b2"}),
			Entry("names", "names=b1,b2", payloads.ServicePlanList{Names: "b1,b2"}),
			Entry("service offering names", "service_offering_names=so1,so2", payloads.ServicePlanList{ServiceOfferingNames: "so1,so2"}),
			Entry("available", "available=true", payloads.ServicePlanList{Available: tools.PtrTo(true)}),
			Entry("not available", "available=false", payloads.ServicePlanList{Available: tools.PtrTo(false)}),
			Entry("broker names", "service_broker_names=b1,b2", payloads.ServicePlanList{BrokerNames: "b1,b2"}),
			Entry("broker guids", "service_broker_guids=b1,b2", payloads.ServicePlanList{BrokerGUIDs: "b1,b2"}),
			Entry("include", "include=service_offering&include=space.organization", payloads.ServicePlanList{
				IncludeResourceRules: []params.IncludeResourceRule{
					{
						RelationshipPath: []string{"service_offering"},
						Fields:           []string{},
					},
					{
						RelationshipPath: []string{"space", "organization"},
						Fields:           []string{},
					},
				},
			}),
			Entry("service broker fields", "fields[service_offering.service_broker]=guid,name", payloads.ServicePlanList{
				IncludeResourceRules: []params.IncludeResourceRule{{
					RelationshipPath: []string{"service_offering", "service_broker"},
					Fields:           []string{"guid", "name"},
				}},
			}),
		)

		DescribeTable("invalid query",
			func(query string, matchError types.GomegaMatcher) {
				_, decodeErr := decodeQuery[payloads.ServicePlanList](query)
				Expect(decodeErr).To(matchError)
			},
			Entry("invalid available", "available=invalid", MatchError(ContainSubstring("failed to parse"))),
			Entry("invalid include", "include=foo", MatchError(ContainSubstring("value must be one of"))),
			Entry("invalid service broker fields", "fields[service_offering.service_broker]=foo", MatchError(ContainSubstring("value must be one of"))),
		)

		Describe("ToMessage", func() {
			It("converts payload to repository message", func() {
				payload := payloads.ServicePlanList{
					ServiceOfferingGUIDs: "b1,b2",
					BrokerGUIDs:          "b1-guid,b2-guid",
					BrokerNames:          "br1,br2",
					Names:                "n1,n2",
					ServiceOfferingNames: "so1,so2",
					Available:            tools.PtrTo(true),
				}
				Expect(payload.ToMessage()).To(Equal(repositories.ListServicePlanMessage{
					ServiceOfferingGUIDs: []string{"b1", "b2"},
					BrokerNames:          []string{"br1", "br2"},
					BrokerGUIDs:          []string{"b1-guid", "b2-guid"},
					Names:                []string{"n1", "n2"},
					ServiceOfferingNames: []string{"so1", "so2"},
					Available:            tools.PtrTo(true),
				}))
			})
		})
	})

	Describe("Visibility", func() {
		Describe("Validation", func() {
			admin := payloads.ServicePlanVisibility{
				Type: korifiv1alpha1.AdminServicePlanVisibilityType,
			}
			adminWithOrgs := payloads.ServicePlanVisibility{
				Type: korifiv1alpha1.AdminServicePlanVisibilityType,
				Organizations: []services.VisibilityOrganization{{
					GUID: "foo",
				}},
			}

			public := payloads.ServicePlanVisibility{
				Type: korifiv1alpha1.PublicServicePlanVisibilityType,
			}
			publicWithOrgs := payloads.ServicePlanVisibility{
				Type: korifiv1alpha1.PublicServicePlanVisibilityType,
				Organizations: []services.VisibilityOrganization{{
					GUID: "foo",
				}},
			}

			org := payloads.ServicePlanVisibility{
				Type: korifiv1alpha1.OrganizationServicePlanVisibilityType,
				Organizations: []services.VisibilityOrganization{{
					GUID: "foo",
				}},
			}
			orgNoOrgs := payloads.ServicePlanVisibility{
				Type: korifiv1alpha1.OrganizationServicePlanVisibilityType,
			}

			invalidType := payloads.ServicePlanVisibility{Type: "invalid"}

			DescribeTable("Validation",
				func(payload payloads.ServicePlanVisibility, validationMatcher types.GomegaMatcher) {
					Expect(payload.Validate()).To(validationMatcher)
				},

				Entry("invalid type", invalidType, MatchError(ContainSubstring("type: value must be one of"))),
				Entry("admin", admin, Not(HaveOccurred())),
				Entry("admin with orgs", adminWithOrgs, MatchError(ContainSubstring("organizations: must be blank"))),
				Entry("public", public, Not(HaveOccurred())),
				Entry("public with orgs", publicWithOrgs, MatchError(ContainSubstring("organizations: must be blank"))),
				Entry("org", org, Not(HaveOccurred())),
				Entry("org no orgs", orgNoOrgs, MatchError(ContainSubstring("organizations: cannot be blank"))),
			)
		})

		Describe("ToMessage", func() {
			var payload *payloads.ServicePlanVisibility

			BeforeEach(func() {
				payload = &payloads.ServicePlanVisibility{
					Type:          korifiv1alpha1.OrganizationServicePlanVisibilityType,
					Organizations: []services.VisibilityOrganization{{GUID: "org-guid"}},
				}
			})

			Describe("ToApplyMessage", func() {
				It("converts payload to repository message", func() {
					Expect(payload.ToApplyMessage("plan-guid")).To(Equal(repositories.ApplyServicePlanVisibilityMessage{
						PlanGUID:      "plan-guid",
						Type:          korifiv1alpha1.OrganizationServicePlanVisibilityType,
						Organizations: []string{"org-guid"},
					}))
				})
			})

			Describe("ToUpdateMessage", func() {
				It("converts payload to repository message", func() {
					Expect(payload.ToUpdateMessage("plan-guid")).To(Equal(repositories.UpdateServicePlanVisibilityMessage{
						PlanGUID:      "plan-guid",
						Type:          korifiv1alpha1.OrganizationServicePlanVisibilityType,
						Organizations: []string{"org-guid"},
					}))
				})
			})
		})
	})
})
