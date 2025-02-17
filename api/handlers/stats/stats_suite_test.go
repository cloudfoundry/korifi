package stats_test

import (
	"context"
	"testing"

	"code.cloudfoundry.org/korifi/api/authorization"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	ctx      context.Context
	authInfo authorization.Info
)

func TestLogcache(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Stats Suite")
}

var _ = BeforeEach(func() {
	authInfo = authorization.Info{Token: "a-token"}
	ctx = authorization.NewContext(context.Background(), &authInfo)
})
