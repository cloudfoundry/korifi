package event_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEvent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Informer Event Suite")
}

var ctx context.Context

var _ = BeforeEach(func() {
	ctx = context.Background()
})
