package payloads_test

import (
	"bytes"
	"encoding/json"
	"net/http"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("Metadata", func() {
	var (
		metadataPayload        payloads.Metadata
		decodedMetadataPayload *payloads.Metadata
		validatorErr           error
	)

	BeforeEach(func() {
		decodedMetadataPayload = new(payloads.Metadata)
		metadataPayload = payloads.Metadata{
			Labels: map[string]string{
				"foo": "bar",
			},
			Annotations: map[string]string{
				"example.org/jim": "hello",
			},
		}
	})

	JustBeforeEach(func() {
		body, err := json.Marshal(metadataPayload)
		Expect(err).NotTo(HaveOccurred())

		req, err := http.NewRequest("", "", bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())

		validatorErr = validator.DecodeAndValidateJSONPayload(req, decodedMetadataPayload)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(decodedMetadataPayload).To(gstruct.PointTo(Equal(metadataPayload)))
	})

	When("labels contains an invalid key", func() {
		BeforeEach(func() {
			metadataPayload.Labels["foo.cloudfoundry.org/bar"] = "jim"
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot use the cloudfoundry.org domain")
		})
	})

	When("annotations contains an invalid key", func() {
		BeforeEach(func() {
			metadataPayload.Annotations["foo.cloudfoundry.org/bar"] = "jim"
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot use the cloudfoundry.org domain")
		})
	})
})

var _ = Describe("MetadataPatch", func() {
	var (
		metadataPatchPayload        payloads.MetadataPatch
		decodedMetadataPatchPayload *payloads.MetadataPatch
		validatorErr                error
	)

	BeforeEach(func() {
		decodedMetadataPatchPayload = new(payloads.MetadataPatch)
		metadataPatchPayload = payloads.MetadataPatch{
			Labels: map[string]*string{
				"foo": tools.PtrTo("bar"),
			},
			Annotations: map[string]*string{
				"example.org/jim": tools.PtrTo("hello"),
			},
		}
	})

	JustBeforeEach(func() {
		body, err := json.Marshal(metadataPatchPayload)
		Expect(err).NotTo(HaveOccurred())

		req, err := http.NewRequest("", "", bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())

		validatorErr = validator.DecodeAndValidateJSONPayload(req, decodedMetadataPatchPayload)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(decodedMetadataPatchPayload).To(gstruct.PointTo(Equal(metadataPatchPayload)))
	})

	When("metadata.labels contains an invalid key", func() {
		BeforeEach(func() {
			metadataPatchPayload.Labels["foo.cloudfoundry.org/bar"] = tools.PtrTo("jim")
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot use the cloudfoundry.org domain")
		})
	})

	When("metadata.annotations contains an invalid key", func() {
		BeforeEach(func() {
			metadataPatchPayload.Annotations["foo.cloudfoundry.org/bar"] = tools.PtrTo("jim")
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot use the cloudfoundry.org domain")
		})
	})
})
