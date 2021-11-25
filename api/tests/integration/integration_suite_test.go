package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/tests/integration/helpers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const oidcPrefix string = "oidc:"

var (
	testEnv      *envtest.Environment
	k8sClient    client.Client
	k8sConfig    *rest.Config
	authProvider *helpers.AuthProvider
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	SetDefaultEventuallyTimeout(10 * time.Second)
})

var _ = BeforeEach(func() {
	authProvider = helpers.NewAuthProvider()
	startEnvTest(authProvider.APIServerExtraArgs(oidcPrefix))
})

var _ = AfterEach(func() {
	authProvider.Stop()
	Expect(testEnv.Stop()).To(Succeed())
})

func startEnvTest(apiServerExtraArgs map[string]string) {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		AttachControlPlaneOutput: false, // set to true for full apiserver logs
	}

	for key, value := range apiServerExtraArgs {
		testEnv.ControlPlane.GetAPIServer().Configure().Append(key, value)
	}

	var err error
	k8sConfig, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	err = authv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(k8sConfig, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	namespaceList := &corev1.NamespaceList{}
	Eventually(func() error {
		return k8sClient.List(context.Background(), namespaceList)
	}).Should(Succeed())

	Eventually(func() error {
		token := authProvider.GenerateJWTToken("ping")
		cfg := rest.AnonymousClientConfig(k8sConfig)
		cfg.BearerToken = token

		_, err := client.New(cfg, client.Options{})
		return err
	}).Should(Succeed())
}

func restartEnvTest(apiServerEtraArgs map[string]string) {
	Expect(testEnv.Stop()).To(Succeed())
	startEnvTest(apiServerEtraArgs)
}

func obtainClientCert(name string) ([]byte, []byte) {
	authUser, err := testEnv.ControlPlane.AddUser(envtest.User{Name: name}, k8sConfig)
	Expect(err).NotTo(HaveOccurred())

	userConfig := authUser.Config()
	return userConfig.CertData, userConfig.KeyData
}

func encodeCertAndKey(certData, keyData []byte) string {
	authHeader := []byte{}
	authHeader = append(authHeader, certData...)
	authHeader = append(authHeader, keyData...)

	return base64.StdEncoding.EncodeToString(authHeader)
}

func generateUnsignedCert(name string) string {
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1658),
		Subject: pkix.Name{
			CommonName: name,
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	privKey, err := rsa.GenerateKey(rand.Reader, 4096)
	Expect(err).NotTo(HaveOccurred())

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, cert, &privKey.PublicKey, privKey)
	Expect(err).NotTo(HaveOccurred())

	buf := new(bytes.Buffer)
	Expect(pem.Encode(buf, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})).To(Succeed())
	Expect(pem.Encode(buf, &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	})).To(Succeed())

	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
