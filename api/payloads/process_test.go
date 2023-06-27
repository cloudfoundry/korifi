package payloads_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("ProcessList", func() {
	Describe("decodes from url values", func() {
		It("succeeds", func() {
			processList := payloads.ProcessList{}
			req, err := http.NewRequest("GET", "http://foo.com/bar?app_guids=app_guid", nil)
			Expect(err).NotTo(HaveOccurred())
			err = validator.DecodeAndValidateURLValues(req, &processList)

			Expect(err).NotTo(HaveOccurred())
			Expect(processList).To(Equal(payloads.ProcessList{
				AppGUIDs: "app_guid",
			}))
		})
	})
})

var _ = Describe("Process payload validation", func() {
	var validatorErr error

	Describe("ProcessScale", func() {
		var (
			payload        payloads.ProcessScale
			decodedPayload *payloads.ProcessScale
		)

		BeforeEach(func() {
			payload = payloads.ProcessScale{
				Instances: tools.PtrTo(1),
				MemoryMB:  tools.PtrTo[int64](2),
				DiskMB:    tools.PtrTo[int64](3),
			}

			decodedPayload = new(payloads.ProcessScale)
		})

		JustBeforeEach(func() {
			validatorErr = validator.DecodeAndValidateJSONPayload(createRequest(payload), decodedPayload)
		})

		It("succeeds", func() {
			Expect(validatorErr).NotTo(HaveOccurred())
			Expect(decodedPayload).To(gstruct.PointTo(Equal(payload)))
		})

		When("instances is negative", func() {
			BeforeEach(func() {
				payload.Instances = tools.PtrTo(-1)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "instances must be 0 or greater")
			})
		})

		When("memory is negative", func() {
			BeforeEach(func() {
				payload.MemoryMB = tools.PtrTo[int64](-1)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "memory_in_mb must be greater than 0")
			})
		})

		When("disk is negative", func() {
			BeforeEach(func() {
				payload.DiskMB = tools.PtrTo[int64](-1)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError(validatorErr, "disk_in_mb must be greater than 0")
			})
		})
	})
})
