package actions_test

import (
	"context"
	"testing"

	"github.com/matt-royal/biloba"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var (
	ctx context.Context
)

func TestApis(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "Actions Suite", biloba.GoLandReporter())
}

var _ = BeforeEach(func() {
	ctx = context.Background()
})
