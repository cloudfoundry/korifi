package actions_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var ctx context.Context

func TestApis(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Actions Suite")
}

var _ = BeforeEach(func() {
	ctx = context.Background()
})
