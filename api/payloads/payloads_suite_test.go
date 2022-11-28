package payloads_test

import (
	"testing"

	"code.cloudfoundry.org/korifi/api/apierrors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPayloads(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Payloads Suite")
}

func expectUnprocessableEntityError(err error, detail string) {
	Expect(err).To(HaveOccurred())
	Expect(err).To(BeAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
	Expect(err.(apierrors.UnprocessableEntityError).Detail()).To(ContainSubstring(detail))
}

func expectUnknownKeyError(err error, detail string) {
	Expect(err).To(HaveOccurred())
	Expect(err).To(BeAssignableToTypeOf(apierrors.UnknownKeyError{}))
	Expect(err.(apierrors.UnknownKeyError).Detail()).To(ContainSubstring(detail))
}
