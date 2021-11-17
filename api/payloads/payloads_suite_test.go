package payloads_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestPayloads(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Payloads Suite")
}
