package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
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
	)

	DescribeTable("invalid query",
		func(query string, expectedErrMsg string) {
			_, decodeErr := decodeQuery[payloads.DropletList](query)
			Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
		},
		Entry("invalid parameter", "foo=bar", "unsupported query parameter: foo"),
	)
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
