package params_test

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads/params"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

var _ = Describe("IncludeResourceRules", func() {
	DescribeTable("ParseFields",
		func(query string, match types.GomegaMatcher) {
			queryValues, err := url.ParseQuery(query)
			Expect(err).NotTo(HaveOccurred())

			Expect(params.ParseFields(queryValues)).To(match)
		},
		Entry("unrelated query", "foo=bar", BeEmpty()),

		Entry("fields query", "fields[space.organization]=name,guid&fields[space]=name", ConsistOf(
			params.IncludeResourceRule{
				RelationshipPath: []string{"space", "organization"},
				Fields:           []string{"name", "guid"},
			},
			params.IncludeResourceRule{
				RelationshipPath: []string{"space"},
				Fields:           []string{"name"},
			}),
		),
	)

	DescribeTable("ParseIncludes",
		func(query string, match types.GomegaMatcher) {
			queryValues, err := url.ParseQuery(query)
			Expect(err).NotTo(HaveOccurred())

			Expect(params.ParseIncludes(queryValues)).To(match)
		},
		Entry("unrelated query", "foo=bar", BeEmpty()),

		Entry("includes query", "include=space.organization&include=service_broker", ConsistOf(
			params.IncludeResourceRule{
				RelationshipPath: []string{"space", "organization"},
				Fields:           []string{},
			},
			params.IncludeResourceRule{
				RelationshipPath: []string{"service_broker"},
				Fields:           []string{},
			}),
		),
	)
})
