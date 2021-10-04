package apis_test

import (
	"testing"

	"github.com/matt-royal/biloba"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestApis(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "Apis Suite", biloba.GoLandReporter())
}
