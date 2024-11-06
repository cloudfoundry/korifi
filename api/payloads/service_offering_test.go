package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/payloads/params"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

var _ = Describe("ServiceOfferingGet", func() {
	DescribeTable("valid query",
		func(query string, expectedServiceOfferingGet payloads.ServiceOfferingGet) {
			actualServiceOfferingGet, decodeErr := decodeQuery[payloads.ServiceOfferingGet](query)

			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(*actualServiceOfferingGet).To(Equal(expectedServiceOfferingGet))
		},
		Entry("fields[service_broker]", "fields[service_broker]=guid,name", payloads.ServiceOfferingGet{
			IncludeResourceRules: []params.IncludeResourceRule{{
				RelationshipPath: []string{"service_broker"},
				Fields:           []string{"guid", "name"},
			}},
		}),
	)

	DescribeTable("invalid query",
		func(query string, errMatcher types.GomegaMatcher) {
			_, decodeErr := decodeQuery[payloads.ServiceOfferingGet](query)
			Expect(decodeErr).To(errMatcher)
		},
		Entry("invalid service broker field", "fields[service_broker]=foo", MatchError(ContainSubstring("value must be one of"))),
		Entry("invalid fields", "fields[space]=foo", MatchError(ContainSubstring("unsupported query parameter: fields[space]"))),
	)
})

var _ = Describe("ServiceOfferingList", func() {
	DescribeTable("valid query",
		func(query string, expectedServiceOfferingList payloads.ServiceOfferingList) {
			actualServiceOfferingList, decodeErr := decodeQuery[payloads.ServiceOfferingList](query)

			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(*actualServiceOfferingList).To(Equal(expectedServiceOfferingList))
		},
		Entry("names", "names=b1,b2", payloads.ServiceOfferingList{Names: "b1,b2"}),
		Entry("service_broker_names", "service_broker_names=b1,b2", payloads.ServiceOfferingList{BrokerNames: "b1,b2"}),
		Entry("fields[service_broker]", "fields[service_broker]=guid,name", payloads.ServiceOfferingList{IncludeBrokerFields: []string{"guid", "name"}}),
	)

	DescribeTable("invalid query",
		func(query string, errMatcher types.GomegaMatcher) {
			_, decodeErr := decodeQuery[payloads.ServiceOfferingList](query)
			Expect(decodeErr).To(errMatcher)
		},
		Entry("invalid service broker field", "fields[service_broker]=foo", MatchError(ContainSubstring("value must be one of: guid, name"))),
	)

	Describe("ToMessage", func() {
		It("converts payload to repository message", func() {
			payload := &payloads.ServiceOfferingList{Names: "b1,b2", BrokerNames: "br1,br2"}

			Expect(payload.ToMessage()).To(Equal(repositories.ListServiceOfferingMessage{
				Names:       []string{"b1", "b2"},
				BrokerNames: []string{"br1", "br2"},
			}))
		})
	})
})
