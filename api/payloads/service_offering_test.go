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
		Entry("created_at", "order_by=created_at", payloads.ServiceOfferingList{OrderBy: "created_at"}),
		Entry("-created_at", "order_by=-created_at", payloads.ServiceOfferingList{OrderBy: "-created_at"}),
		Entry("updated_at", "order_by=updated_at", payloads.ServiceOfferingList{OrderBy: "updated_at"}),
		Entry("-updated_at", "order_by=-updated_at", payloads.ServiceOfferingList{OrderBy: "-updated_at"}),
		Entry("name", "order_by=name", payloads.ServiceOfferingList{OrderBy: "name"}),
		Entry("-name", "order_by=-name", payloads.ServiceOfferingList{OrderBy: "-name"}),
		Entry("service_broker_names", "service_broker_names=b1,b2", payloads.ServiceOfferingList{BrokerNames: "b1,b2"}),
		Entry("fields[service_broker]", "fields[service_broker]=guid,name", payloads.ServiceOfferingList{
			IncludeResourceRules: []params.IncludeResourceRule{{
				RelationshipPath: []string{"service_broker"},
				Fields:           []string{"guid", "name"},
			}},
		}),
	)

	DescribeTable("invalid query",
		func(query string, errMatcher types.GomegaMatcher) {
			_, decodeErr := decodeQuery[payloads.ServiceOfferingList](query)
			Expect(decodeErr).To(errMatcher)
		},
		Entry("invalid service broker field", "fields[service_broker]=foo", MatchError(ContainSubstring("value must be one of"))),

		Entry("invalid order_by", "order_by=foo", MatchError(ContainSubstring("value must be one of"))),
		Entry("per_page is not a number", "per_page=foo", MatchError(ContainSubstring("value must be an integer"))),
	)

	Describe("ToMessage", func() {
		It("converts payload to repository message", func() {
			payload := &payloads.ServiceOfferingList{
				Names:       "b1,b2",
				BrokerNames: "br1,br2",
				OrderBy:     "created_at",
				Pagination: payloads.Pagination{
					PerPage: "10",
					Page:    "2",
				},
			}

			Expect(payload.ToMessage()).To(Equal(repositories.ListServiceOfferingMessage{
				Names:       []string{"b1", "b2"},
				BrokerNames: []string{"br1", "br2"},
				OrderBy:     "created_at",
				Pagination: repositories.Pagination{
					PerPage: 10,
					Page:    2,
				},
			}))
		})
	})
})

var _ = Describe("ServiceOfferingDelete", func() {
	DescribeTable("valid query",
		func(query string, expectedServiceOfferingDelete payloads.ServiceOfferingDelete) {
			actualServiceOfferingDelete, decodeErr := decodeQuery[payloads.ServiceOfferingDelete](query)

			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(*actualServiceOfferingDelete).To(Equal(expectedServiceOfferingDelete))
		},
		Entry("purge", "purge=true", payloads.ServiceOfferingDelete{Purge: true}),
	)

	DescribeTable("invalid query",
		func(query string, expectedErrMsg string) {
			_, decodeErr := decodeQuery[payloads.ServiceOfferingDelete](query)
			Expect(decodeErr).To(HaveOccurred())
		},
		Entry("unsuported param", "foo=bar", "unsupported query parameter: foo"),
		Entry("invalid value for purge", "purge=foo", "invalid syntax"),
	)
})
