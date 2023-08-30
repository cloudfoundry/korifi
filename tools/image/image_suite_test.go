package image_test

import (
	"context"
	"os"
	"testing"

	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tests/helpers/oci"
	"code.cloudfoundry.org/korifi/tools/dockercfg"
	"code.cloudfoundry.org/korifi/tools/image"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
	registries = []registry{}
	registriesEnv, ok := os.LookupEnv("REGISTRY_DETAILS")
	if ok {
		err := yaml.Unmarshal([]byte(registriesEnv), &registries)
		NewWithT(t).Expect(err).NotTo(HaveOccurred())
	}

	RegisterFailHandler(Fail)
	RunSpecs(t, "Image Suite")
}

var (
	imgClient          image.Client
	k8sClient          client.Client
	k8sClientset       *kubernetes.Clientset
	k8sConfig          *rest.Config
	containerRegistry  *oci.Registry
	testEnv            *envtest.Environment
	ctx                context.Context
	registries         []registry
	secretName         string
	serviceAccountName string
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

	k8sClient, err = client.NewWithWatch(k8sConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	k8sClientset, err = kubernetes.NewForConfig(k8sConfig)
	Expect(err).NotTo(HaveOccurred())

	containerRegistry = oci.NewContainerRegistry("user", "password")

	secretName = testutils.GenerateGUID()

	dockerConfigs := []dockercfg.DockerServerConfig{}
	for _, reg := range registries {
		dockerConfigs = append(dockerConfigs, dockercfg.DockerServerConfig{
			Server:   reg.Server,
			Username: reg.Username,
			Password: reg.Password,
		})
	}
	dockerConfigs = append(dockerConfigs, dockercfg.DockerServerConfig{
		Server:   containerRegistry.URL(),
		Username: "user",
		Password: "password",
	})

	dockerSecret, err := dockercfg.CreateDockerConfigSecret("default", secretName, dockerConfigs...)
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient.Create(ctx, dockerSecret)).To(Succeed())

	Eventually(func(g Gomega) {
		_, getErr := k8sClientset.CoreV1().Secrets("default").Get(ctx, secretName, metav1.GetOptions{})
		g.Expect(getErr).NotTo(HaveOccurred())
	}).Should(Succeed())

	serviceAccountName = testutils.GenerateGUID()
	Expect(k8sClient.Create(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      serviceAccountName,
		},
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: secretName}},
	})).To(Succeed())
})

var _ = AfterSuite(func() {
	if k8sConfig != nil {
		Expect(testEnv.Stop()).To(Succeed())
	}
})
