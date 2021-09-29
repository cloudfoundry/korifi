//go:build e2e
// +build e2e

package e2e_test

import (
	"net/http"
	"os"
	"testing"

	"github.com/hashicorp/go-uuid"
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	controllerruntime "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	hncv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var (
	suite             spec.Suite
	testServerAddress string
	g                 *WithT
	k8sClient         client.Client
	rootNamespace     string
	apiServerRoot     string
)

func Suite() spec.Suite {
	if suite == nil {
		suite = spec.New("E2E Tests")
	}

	return suite
}

func SuiteDescribe(desc string, f func(t *testing.T, when spec.G, it spec.S)) bool {
	return Suite()(desc, f)
}

func TestSuite(t *testing.T) {
	g = NewWithT(t)

	beforeSuite()
	defer afterSuite()

	suite.Run(t)
}

func beforeSuite() {
	apiServerRoot = mustHaveEnv("API_SERVER_ROOT")

	logf.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.UseDevMode(true)))

	hncv1alpha2.AddToScheme(scheme.Scheme)

	config, err := controllerruntime.GetConfig()
	g.Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(config, client.Options{Scheme: scheme.Scheme})
	g.Expect(err).NotTo(HaveOccurred())

	rootNamespace = mustHaveEnv("ROOT_NAMESPACE")
	ensureServerIsUp()
}

func afterSuite() {
}

func mustHaveEnv(key string) string {
	val, ok := os.LookupEnv(key)
	g.Expect(ok).To(BeTrue(), "must set env var %q", key)

	return val
}

func ensureServerIsUp() {
	g.Eventually(func() (int, error) {
		resp, err := http.Get(apiServerRoot)
		if err != nil {
			return 0, err
		}

		resp.Body.Close()

		return resp.StatusCode, nil
	}, "30s").Should(Equal(http.StatusOK), "API Server at %s was not running after 30 seconds", apiServerRoot)
}

func generateGUID() string {
	guid, err := uuid.GenerateUUID()
	g.Expect(err).NotTo(HaveOccurred())

	return guid[:30]
}
