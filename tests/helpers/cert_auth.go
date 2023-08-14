package helpers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"

	"code.cloudfoundry.org/korifi/tools"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
	certv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateSelfSignedCertificatePEM() []byte {
	certPrivKey := generateRSAKey()

	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"cf-on-k8s"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(time.Hour * 24 * 180),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, cert, &certPrivKey.PublicKey, certPrivKey)
	Expect(err).NotTo(HaveOccurred())

	keyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})

	certPem := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	return append(certPem, keyPem...)
}

func CreateTrustedCertificatePEM(userName string, validFor time.Duration) []byte {
	certPrivKey := generateRSAKey()
	certPem := generateTrustedCertificate(certPrivKey, userName, validFor)

	keyPem := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})

	return append(certPem, keyPem...)
}

func generateRSAKey() *rsa.PrivateKey {
	GinkgoHelper()

	key, err := rsa.GenerateKey(rand.Reader, 4096)
	Expect(err).NotTo(HaveOccurred())

	return key
}

func generateTrustedCertificate(key *rsa.PrivateKey, userName string, validFor time.Duration) []byte {
	GinkgoHelper()

	config, err := controllerruntime.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	Expect(certv1.AddToScheme(scheme.Scheme)).To(Succeed())
	k8sClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	csr := generateCSR(k8sClient, key, userName, validFor)
	defer func() {
		err := k8sClient.Delete(context.Background(), csr)
		if err != nil {
			fmt.Printf("failed to delete csr %q: %v", csr.Name, err)
		}
	}()

	approveCSR(csr)

	Eventually(func(g Gomega) {
		g.Expect(k8sClient.Get(context.Background(), client.ObjectKeyFromObject(csr), csr)).To(Succeed())

		g.Expect(csr.Status.Certificate).NotTo(BeEmpty())
	}).WithTimeout(EventuallyTimeout()).Should(Succeed())

	return csr.Status.Certificate
}

func generateCSR(k8sClient client.Client, key *rsa.PrivateKey, userName string, validFor time.Duration) *certv1.CertificateSigningRequest {
	GinkgoHelper()

	subject := pkix.Name{
		CommonName: userName,
	}

	createCertificateRequest, err := x509.CreateCertificateRequest(
		rand.Reader,
		&x509.CertificateRequest{
			Subject: subject,
			// DNSNames: []string{commonName},
		},
		key,
	)
	Expect(err).NotTo(HaveOccurred())

	certRequest := pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE REQUEST", Bytes: createCertificateRequest,
	})

	Expect(certv1.AddToScheme(scheme.Scheme)).To(Succeed())

	csr := &certv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: uuid.NewString(),
		},
		Spec: certv1.CertificateSigningRequestSpec{
			Request:           certRequest,
			SignerName:        "kubernetes.io/kube-apiserver-client",
			ExpirationSeconds: tools.PtrTo(int32(validFor.Seconds())),
			Usages:            []certv1.KeyUsage{"client auth"},
		},
	}
	err = k8sClient.Create(context.Background(), csr)
	Expect(err).NotTo(HaveOccurred())

	return csr
}

func approveCSR(csr *certv1.CertificateSigningRequest) {
	GinkgoHelper()

	config, err := controllerruntime.GetConfig()
	Expect(err).NotTo(HaveOccurred())
	clientSet, err := kubernetes.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred())

	csr.Status.Conditions = append(csr.Status.Conditions, certv1.CertificateSigningRequestCondition{
		Type:           certv1.RequestConditionType(certv1.CertificateApproved),
		Status:         corev1.ConditionTrue,
		Reason:         "e2e-approved",
		Message:        "approved by e2e suite test",
		LastUpdateTime: metav1.Now(),
	})

	_, err = clientSet.CertificatesV1().CertificateSigningRequests().UpdateApproval(
		context.Background(),
		csr.Name,
		csr,
		metav1.UpdateOptions{},
	)
	Expect(err).NotTo(HaveOccurred())
}
