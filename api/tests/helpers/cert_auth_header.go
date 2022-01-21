package helpers

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"time"

	. "github.com/onsi/gomega"
)

func CreateCertificateAuthHeader() string {
	certKeyPEM := CreateCertificatePEM()

	return "ClientCert " + base64.StdEncoding.EncodeToString(certKeyPEM)
}

func CreateCertificatePEM() []byte {
	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	Expect(err).NotTo(HaveOccurred())

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

	out := new(bytes.Buffer)
	err = pem.Encode(out, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})
	Expect(err).NotTo(HaveOccurred())

	err = pem.Encode(out, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})
	Expect(err).NotTo(HaveOccurred())

	return out.Bytes()
}
