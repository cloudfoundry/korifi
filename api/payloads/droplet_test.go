package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/onsi/gomega/gstruct"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DropletList", func() {
	DescribeTable("valid query",
		func(query string, expectedDropletList payloads.DropletList) {
			actualDropletList, decodeErr := decodeQuery[payloads.DropletList](query)

			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(*actualDropletList).To(Equal(expectedDropletList))
		},
		Entry("guids", "guids=guid", payloads.DropletList{GUIDs: "guid"}),
		Entry("app_guids", "app_guids=guid", payloads.DropletList{AppGUIDs: "guid"}),
		Entry("space_guids", "space_guids=guid", payloads.DropletList{SpaceGUIDs: "guid"}),
		Entry("order_by created_at", "order_by=created_at", payloads.DropletList{OrderBy: "created_at"}),
		Entry("order_by -created_at", "order_by=-created_at", payloads.DropletList{OrderBy: "-created_at"}),
		Entry("order_by updated_at", "order_by=updated_at", payloads.DropletList{OrderBy: "updated_at"}),
		Entry("order_by -updated_at", "order_by=-updated_at", payloads.DropletList{OrderBy: "-updated_at"}),
		Entry("pagination", "page=3", payloads.DropletList{Pagination: payloads.Pagination{Page: "3"}}),
	)

	DescribeTable("invalid query",
		func(query string, expectedErrMsg string) {
			_, decodeErr := decodeQuery[payloads.DropletList](query)
			Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
		},
		Entry("invalid order_by", "order_by=foo", "value must be one of"),
		Entry("invalid parameter", "foo=bar", "unsupported query parameter: foo"),
		Entry("invalid pagination", "per_page=foo", "value must be an integer"),
	)

	Describe("ToMessage", func() {
		It("translates to repo message", func() {
			dropletList := payloads.DropletList{
				GUIDs:      "g1,g2",
				AppGUIDs:   "ag1,ag2",
				SpaceGUIDs: "sg1,sg2",
				OrderBy:    "created_at",
				Pagination: payloads.Pagination{
					PerPage: "3",
					Page:    "2",
				},
			}
			Expect(dropletList.ToMessage()).To(Equal(repositories.ListDropletsMessage{
				GUIDs:      []string{"g1", "g2"},
				AppGUIDs:   []string{"ag1", "ag2"},
				SpaceGUIDs: []string{"sg1", "sg2"},
				OrderBy:    "created_at",
				Pagination: repositories.Pagination{
					Page:    2,
					PerPage: 3,
				},
			}))
		})
	})
})

var _ = Describe("DropletUpdate", func() {
	Describe("Decode", func() {
		var (
			updatePayload         payloads.DropletUpdate
			decodedDropletPayload *payloads.DropletUpdate
			validatorErr          error
		)

		BeforeEach(func() {
			decodedDropletPayload = new(payloads.DropletUpdate)
			updatePayload = payloads.DropletUpdate{
				Metadata: payloads.MetadataPatch{
					Labels: map[string]*string{
						"foo": tools.PtrTo("bar"),
					},
					Annotations: map[string]*string{
						"example.org/jim": tools.PtrTo("hello"),
					},
				},
			}
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(updatePayload), decodedDropletPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedDropletPayload).To(gstruct.PointTo(Equal(updatePayload)))
		})

		When("metadata.labels contains an invalid key", func() {
			BeforeEach(func() {
				updatePayload.Metadata = payloads.MetadataPatch{
					Labels: map[string]*string{
						"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
					},
				}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "cannot use the cloudfoundry.org domain")
			})
		})

		When("metadata.annotations contains an invalid key", func() {
			BeforeEach(func() {
				updatePayload.Metadata = payloads.MetadataPatch{
					Annotations: map[string]*string{
						"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
					},
				}
			})

			It("returns an appropriate error", func() {
				expectUnprocessableEntityError(validatorErr, "cannot use the cloudfoundry.org domain")
			})
		})
	})
})
