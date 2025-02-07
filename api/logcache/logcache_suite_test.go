package logcache_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var ctx context.Context

func TestLogcache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Logcache Client Suite")
}

var _ = BeforeEach(func() {
	ctx = context.Background()
})
