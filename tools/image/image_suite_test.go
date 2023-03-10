package image_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"

	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/image"
	"github.com/distribution/distribution/v3/configuration"
	dcontext "github.com/distribution/distribution/v3/context"
	_ "github.com/distribution/distribution/v3/registry/auth/htpasswd"
	"github.com/distribution/distribution/v3/registry/handlers"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestImage(t *testing.T) {
	registries = []registry{
		// {
		// 	Username:   "user",
		// 	Password:   "password",
		// 	Server:     authRegistryServer.URL,
		// 	PathPrefix: strings.Replace(authRegistryServer.URL, "http://", "", 1),
		// },
	}
	registriesEnv, ok := os.LookupEnv("REGISTRY_DETAILS")
	if ok {
		err := yaml.Unmarshal([]byte(registriesEnv), &registries)
		NewWithT(t).Expect(err).NotTo(HaveOccurred())
	}

	RegisterFailHandler(Fail)
	RunSpecs(t, "Image Suite")
}

var (
	imgClient            image.Client
	k8sClient            client.Client
	k8sClientset         *kubernetes.Clientset
	k8sConfig            *rest.Config
	authRegistryServer   *httptest.Server
	noAuthRegistryServer *httptest.Server
	testEnv              *envtest.Environment
	ctx                  context.Context
	registries           []registry
	secretName           string
)

type registry struct {
	Server     string `yaml:"server"`
	PathPrefix string `yaml:"pathPrefix"`
	RepoName   string `yaml:"repoName"`
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx = context.Background()

	testEnv = &envtest.Environment{}

	var err error
	k8sConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	logger.SetOutput(GinkgoWriter)
	dcontext.SetDefaultLogger(logrus.NewEntry(logger))

	k8sClient, err = client.NewWithWatch(k8sConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	k8sClientset, err = kubernetes.NewForConfig(k8sConfig)
	Expect(err).NotTo(HaveOccurred())

	authRegistryServer = httptest.NewServer(handlers.NewApp(ctx, &configuration.Configuration{
		Auth: configuration.Auth{
			"htpasswd": configuration.Parameters{
				"realm": "Registry Realm",
				"path":  "fixtures/htpasswd", // user:password
			},
		},
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
			"delete":   configuration.Parameters{"enabled": true},
		},
		Loglevel: "debug",
	}))

	noAuthRegistryServer = httptest.NewServer(handlers.NewApp(ctx, &configuration.Configuration{
		Storage: configuration.Storage{
			"inmemory": configuration.Parameters{},
			"delete":   configuration.Parameters{"enabled": true},
		},
		Loglevel: "debug",
	}))

	secretName = testutils.GenerateGUID()
	dockerConfig := map[string]map[string]any{"auths": {}}
	for _, reg := range registries {
		dockerConfig["auths"][reg.Server] = map[string]string{
			"username": reg.Username,
			"password": reg.Password,
			"auth":     base64.StdEncoding.EncodeToString([]byte(reg.Username + ":" + reg.Password)),
		}
	}
	dockerConfig["auths"][authRegistryServer.URL] = map[string]string{
		"username": "user",
		"password": "password",
		"auth":     base64.StdEncoding.EncodeToString([]byte("user:password")),
	}

	dockerConfigJSON, err := json.Marshal(dockerConfig)
	Expect(err).NotTo(HaveOccurred())

	Expect(k8sClient.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "default",
		},
		Data: map[string][]byte{
			".dockerconfigjson": dockerConfigJSON,
		},
		Type: "kubernetes.io/dockerconfigjson",
	})).To(Succeed())
	Eventually(func(g Gomega) {
		_, getErr := k8sClientset.CoreV1().Secrets("default").Get(ctx, secretName, metav1.GetOptions{})
		g.Expect(getErr).NotTo(HaveOccurred())
	}).Should(Succeed())
})

var _ = AfterSuite(func() {
	if authRegistryServer != nil {
		authRegistryServer.Close()
	}

	if noAuthRegistryServer != nil {
		noAuthRegistryServer.Close()
	}

	if k8sConfig != nil {
		Expect(testEnv.Stop()).To(Succeed())
	}
})
