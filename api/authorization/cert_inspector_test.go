package authorization_test

import (
	"bytes"
	"context"
	"encoding/pem"
	"errors"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/authorization/testhelpers"
	"code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("CertInspector", func() {
	var (
		ctx           context.Context
		certInspector *authorization.CertInspector
		id            authorization.Identity
		certPEM       []byte
		inspectorErr  error
	)

	BeforeEach(func() {
		ctx = context.Background()
		certInspector = authorization.NewCertInspector(k8sConfig)
		certData, keyData := testhelpers.ObtainClientCert(testEnv, "alice", "my-group")
		certPEM = certData
		certPEM = append(certPEM, keyData...)
	})

	JustBeforeEach(func() {
		id, inspectorErr = certInspector.WhoAmI(ctx, certPEM)
	})

	It("extracts identity from a valid certificate", func() {
		Expect(inspectorErr).NotTo(HaveOccurred())
		Expect(id.Kind).To(Equal(rbacv1.UserKind))
		Expect(id.Name).To(Equal("alice"))
		Expect(id.Groups).To(ContainElement("my-group"))
	})

	When("the certificate is not recognized by the cluster", func() {
		BeforeEach(func() {
			certPEM = generateUnsignedCert("alice")
		})

		It("returns a InvalidAuthError", func() {
			Expect(inspectorErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.InvalidAuthError{}))
		})
	})

	When("the cert is not PEM encoded", func() {
		BeforeEach(func() {
			certPEM = []byte("foo")
		})

		It("returns an error", func() {
			Expect(inspectorErr).To(Equal(apierrors.NewInvalidAuthError(errors.New("failed to decode cert PEM"))))
		})
	})

	When("the cert contains multiple PEM blocks", func() {
		BeforeEach(func() {
			certPEM = appendBadPEMBlock(certPEM)
		})

		It("uses the first", func() {
			Expect(inspectorErr).NotTo(HaveOccurred())
			Expect(id.Name).To(Equal("alice"))
		})
	})

	When("the cert cannot be parsed", func() {
		BeforeEach(func() {
			certPEM = appendBadPEMBlock([]byte{})
		})

		It("returns an error", func() {
			Expect(inspectorErr).To(SatisfyAll(
				matchers.WrapErrorAssignableToTypeOf(apierrors.InvalidAuthError{}),
				MatchError(ContainSubstring("failed to parse certificate")),
			))
		})
	})
})

func appendBadPEMBlock(certPEM []byte) []byte {
	result := bytes.NewBuffer(certPEM)

	Expect(pem.Encode(result, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("hello"),
	})).To(Succeed())

	return result.Bytes()
}
