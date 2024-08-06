package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/model/services"
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
		)

		Describe("ToMessage", func() {
			It("converts payload to repository message", func() {
				payload := &payloads.ServicePlanList{ServiceOfferingGUIDs: "b1,b2", Names: "n1,n2"}

				Expect(payload.ToMessage()).To(Equal(repositories.ListServicePlanMessage{
					ServiceOfferingGUIDs: []string{"b1", "b2"},
					Names:                []string{"n1", "n2"},
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
