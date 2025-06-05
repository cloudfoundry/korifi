package testhelpers

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/golang-jwt/jwt/v5"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
)

const audience string = "test-audience"

// When configured to work with OIDC the k8s api server calls the
// /.well-known/openid-configuration endpoint of the configurd oidc issuer.
// Later when a token review is created the api server tries to validate the
// token using the public part of the key that the token was signed with. This
// public key is serverd on the jwks_uri endpoint that was advertised by the
// initial request of /.well-known/openid-configuration.
//
// This utility generates JWT tokens and signs them with a signing key, while
// at the same time serving the public part of the signing key to the api
// server.
type AuthProvider struct {
	server       *ghttp.Server
	serverCAPath string
	signingKey   *rsa.PrivateKey
}

func NewAuthProvider() *AuthProvider {
	signingKey, err := rsa.GenerateKey(rand.Reader, 2048)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	server := ghttp.NewTLSServer()
	configureServer(server, signingKey)

	return &AuthProvider{
		server:       server,
		serverCAPath: writeCAToTempFile(server),
		signingKey:   signingKey,
	}
}

func (p *AuthProvider) GenerateJWTToken(subject string, groups ...string) string {
	atClaims := jwt.MapClaims{}
	atClaims["iss"] = p.server.URL()
	atClaims["aud"] = audience
	atClaims["sub"] = subject
	atClaims["exp"] = time.Now().Add(time.Minute * 15).Unix()
	atClaims["groups"] = groups
	at := jwt.NewWithClaims(jwt.SigningMethodRS256, atClaims)
	token, err := at.SignedString(p.signingKey)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return token
}

func writeCAToTempFile(server *ghttp.Server) string {
	caDerBytes := server.HTTPTestServer.TLS.Certificates[0].Certificate[0]

	certOut, err := os.CreateTemp("", "test-oidc-ca")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: caDerBytes})
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	gomega.Expect(certOut.Close()).To(gomega.Succeed())

	return certOut.Name()
}

func renderJWKSResponse(signingKey *rsa.PrivateKey) string {
	template := &x509.Certificate{
		IsCA:                  true,
		BasicConstraintsValid: true,
		SubjectKeyId:          []byte{1, 2, 3},
		SerialNumber:          big.NewInt(1234),
		Subject: pkix.Name{
			Country:      []string{"Earth"},
			Organization: []string{"Mother Nature"},
		},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}

	signingDerBytes, err := x509.CreateCertificate(rand.Reader, template, template, &signingKey.PublicKey, signingKey)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	cert, err := x509.ParseCertificate(signingDerBytes)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	jwks := jose.JSONWebKeySet{
		Keys: []jose.JSONWebKey{{
			Key:          &signingKey.PublicKey,
			KeyID:        "1",
			Use:          "sig",
			Algorithm:    "RS256",
			Certificates: []*x509.Certificate{cert},
		}},
	}
	jwksBytes, err := json.MarshalIndent(jwks, "", "  ")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return string(jwksBytes)
}

func configureServer(server *ghttp.Server, signingKey *rsa.PrivateKey) {
	applicationJSONHeader := http.Header{}
	applicationJSONHeader.Add("Content-Type", "application/json")

	server.SetAllowUnhandledRequests(true)

	server.RouteToHandler(
		http.MethodGet,
		"/.well-known/openid-configuration",
		ghttp.RespondWith(
			http.StatusOK,
			fmt.Sprintf(`{
                   "issuer":
                     "%[1]s",
                   "jwks_uri":
                     "%[1]s/jwks.json"
                }`, server.URL()),
			applicationJSONHeader,
		),
	)

	server.RouteToHandler(
		http.MethodGet,
		"/jwks.json",
		ghttp.RespondWith(
			http.StatusOK,
			renderJWKSResponse(signingKey),
			applicationJSONHeader,
		),
	)
}

func (p *AuthProvider) Stop() {
	p.server.Close()
	gomega.Expect(os.RemoveAll(p.serverCAPath)).To(gomega.Succeed())
}

func (p *AuthProvider) APIServerExtraArgs(oidcPrefix string) map[string]string {
	return map[string]string{
		"oidc-issuer-url":      p.server.URL(),
		"oidc-client-id":       audience,
		"oidc-ca-file":         p.serverCAPath,
		"oidc-username-prefix": oidcPrefix,
		"oidc-groups-claim":    "groups",
	}
}
