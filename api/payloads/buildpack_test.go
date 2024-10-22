package payloads_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
)

var _ = Describe("BuildpackList", func() {
	Describe("Validation", func() {
		DescribeTable("valid query",
			func(query string, expectedBuildpackList payloads.BuildpackList) {
				actualBuildpackList, decodeErr := decodeQuery[payloads.BuildpackList](query)

				Expect(decodeErr).NotTo(HaveOccurred())
				Expect(*actualBuildpackList).To(Equal(expectedBuildpackList))
			},

			Entry("created_at", "order_by=created_at", payloads.BuildpackList{OrderBy: "created_at"}),
			Entry("-created_at", "order_by=-created_at", payloads.BuildpackList{OrderBy: "-created_at"}),
			Entry("updated_at", "order_by=updated_at", payloads.BuildpackList{OrderBy: "updated_at"}),
			Entry("-updated_at", "order_by=-updated_at", payloads.BuildpackList{OrderBy: "-updated_at"}),
			Entry("position", "order_by=position", payloads.BuildpackList{OrderBy: "position"}),
			Entry("-position", "order_by=-position", payloads.BuildpackList{OrderBy: "-position"}),
			Entry("empty", "order_by=", payloads.BuildpackList{OrderBy: ""}),
		)

		DescribeTable("invalid query",
			func(query string, expectedErrMsg string) {
				_, decodeErr := decodeQuery[payloads.BuildpackList](query)
				Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
			},
			Entry("invalid order_by", "order_by=foo", "value must be one of"),
		)
	})

	DescribeTable("ToMessage",
		func(buildpackList payloads.BuildpackList, expectedListBuildpacksMessage repositories.ListBuildpacksMessage) {
			actualListBuildpacksMessage := buildpackList.ToMessage()

			Expect(actualListBuildpacksMessage).To(Equal(expectedListBuildpacksMessage))
		},
		Entry("created_at", payloads.BuildpackList{OrderBy: "created_at"}, repositories.ListBuildpacksMessage{OrderBy: "created_at"}),
	)
})
