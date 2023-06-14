package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/onsi/gomega/gstruct"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

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
			validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(updatePayload), decodedDropletPayload)
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
				expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
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
				expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
			})
		})
	})
})
