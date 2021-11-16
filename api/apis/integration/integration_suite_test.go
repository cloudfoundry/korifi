package integration_test

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/matt-royal/biloba"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecsWithDefaultAndCustomReporters(t, "Integration Suite", biloba.GoLandReporter())
}

var (
	testEnv   *envtest.Environment
	k8sClient client.WithWatch
	k8sConfig *rest.Config
	server    *http.Server
	port      int
)

var (
	rr        *httptest.ResponseRecorder
	req       *http.Request
	router    *mux.Router
	serverURL *url.URL
	ctx       context.Context
)
var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "controllers", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	k8sConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sConfig).NotTo(BeNil())

	err = workloadsv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())
	err = networkingv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.NewWithWatch(k8sConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	rand.Seed(time.Now().UnixNano())
})

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).To(Succeed())
})

var _ = BeforeEach(func() {
	ctx = context.Background()
	rr = httptest.NewRecorder()
	router = mux.NewRouter()

	port = 1024 + rand.Intn(8975)

	serverAddr := fmt.Sprintf("localhost:%d", port)
	var err error
	serverURL, err = url.Parse("http://" + serverAddr)
	Expect(err).NotTo(HaveOccurred())

	server = &http.Server{Addr: serverAddr, Handler: router}
	go func() {
		defer GinkgoRecover()
		Expect(
			server.ListenAndServe(),
		).To(MatchError("http: Server closed"))
	}()
})

var _ = AfterEach(func() {
	Expect(
		server.Close(),
	).To(Succeed())
})

func serverURI(paths ...string) string {
	return fmt.Sprintf("%s%s", serverURL, strings.Join(paths, ""))
}

func generateGUID() string {
	newUUID, err := uuid.NewUUID()
	if err != nil {
		errorMessage := fmt.Sprintf("could not generate a UUID %v", err)
		panic(errorMessage)
	}
	return newUUID.String()
}
