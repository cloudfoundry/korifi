package k8sklient_test

import (
	"context"
	"log"
	"testing"

	"code.cloudfoundry.org/korifi/api/authorization"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
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
	ctx = logr.NewContext(ctx, stdr.New(log.New(GinkgoWriter, ">>>", log.LstdFlags)))
	utilruntime.Must(korifiv1alpha1.AddToScheme(scheme.Scheme))
})
