package payloads_test

import (
	"testing"

	"github.com/matt-royal/biloba"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestPayloads(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "Payloads Suite", biloba.GoLandReporter())
}
