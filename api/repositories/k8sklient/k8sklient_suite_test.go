package k8sklient_test

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

func TestK8sklient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "K8sklient Suite")
}

var _ = BeforeEach(func() {
	authInfo = authorization.Info{
		Token: "i-am-user",
	}
	ctx = authorization.NewContext(context.Background(), &authInfo)
})
