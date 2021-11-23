package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/pem"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("CertInspector", func() {
	var (
		ctx           context.Context
		certInspector authorization.IdentityInspector
		id            authorization.Identity
		certPEMBase64 string
		inspectorErr  error
	)

	BeforeEach(func() {
		ctx = context.Background()
		certInspector = authorization.NewCertInspector(k8sConfig)
		certPEMBase64 = obtainClientCert("alice")
	})

	JustBeforeEach(func() {
		id, inspectorErr = certInspector.WhoAmI(ctx, certPEMBase64)
	})

	It("extracts identity from a valid certificate", func() {
		Expect(inspectorErr).NotTo(HaveOccurred())
		Expect(id.Kind).To(Equal(rbacv1.UserKind))
		Expect(id.Name).To(Equal("alice"))
	})

	When("the certificate is not recognized by the cluster", func() {
		BeforeEach(func() {
			certPEMBase64 = generateUnsignedCert("alice")
		})

		It("returns a InvalidAuthError", func() {
			Expect(inspectorErr).To(HaveOccurred())
			Expect(authorization.IsInvalidAuth(inspectorErr)).To(BeTrue(), "%#v", inspectorErr)
		})
	})

	When("the cert is invalid base64", func() {
		BeforeEach(func() {
			certPEMBase64 = "$*&^^%%"
		})

		It("returns an error", func() {
			Expect(inspectorErr).To(MatchError("failed to base64 decode cert"))
		})
	})

	When("the cert is not PEM encoded", func() {
		BeforeEach(func() {
			certPEMBase64 = "Zm9vCg=="
		})

		It("returns an error", func() {
			Expect(inspectorErr).To(MatchError("failed to decode cert PEM"))
		})
	})

	When("the cert contains multiple PEM blocks", func() {
		BeforeEach(func() {
			certPEMBase64 = appendBadPEMBlock(certPEMBase64)
		})

		It("uses the first", func() {
			Expect(inspectorErr).NotTo(HaveOccurred())
			Expect(id.Name).To(Equal("alice"))
		})
	})

	When("the cert cannot be parsed", func() {
		BeforeEach(func() {
			certPEMBase64 = appendBadPEMBlock("")
		})

		It("returns an error", func() {
			Expect(inspectorErr).To(MatchError(ContainSubstring("failed to parse certificate")))
		})
	})
})

func appendBadPEMBlock(certPEMBase64 string) string {
	clear, err := base64.StdEncoding.DecodeString(certPEMBase64)
	Expect(err).NotTo(HaveOccurred())

	result := bytes.NewBuffer(clear)

	err = pem.Encode(result, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("hello"),
	})
	Expect(err).NotTo(HaveOccurred())

	return base64.StdEncoding.EncodeToString(result.Bytes())
}
