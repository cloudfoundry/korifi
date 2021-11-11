package authorization_test

import (
	"context"
	"encoding/base64"
	"encoding/pem"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
)

var _ = Describe("CertInspector", func() {
	var (
		certInspector authorization.CertInspector
		certPEMBase64 string
		identity      authorization.Identity
		err           error
		ctx           context.Context
	)

	BeforeEach(func() {
		certInspector = authorization.NewCertInspector()
		certPEMBase64 = "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUNmRENDQVdTZ0F3SUJBZ0lRWFRUb3RMZmxJengzQkFEblF2RVMzVEFOQmdrcWhraUc5dzBCQVFzRkFEQVYKTVJNd0VRWURWUVFERXdwcmRXSmxjbTVsZEdWek1CNFhEVEl4TVRFeE1URXdNREkwTWxvWERUSXlNVEV4TVRFdwpNREkwTWxvd0d6RVpNQmNHQTFVRUF4TVFZMlZ5ZEMxMWMyVnlMVEJoTWprNVlqQ0JuekFOQmdrcWhraUc5dzBCCkFRRUZBQU9CalFBd2dZa0NnWUVBMlRVcDFuOGRyOXNRZjJaTFNwRkdCZDl1dy8rUDFlK2poaXozSGhSVjdmWTkKVDZTSlhGUnVxdjV1Yk9nK1pUSVhleDZUTUFOOTdHaktTKzdsWjVlYjl6Rmw1R0xhRStObzY3enJQU3kzbjZvMgpEWTRiTzBRTHQxZUpxdW12LzByOFd2aVhDcHJRM1dNZ2pNSHEvRTZSR00wVzVrU3VqZHlnN0czd2dzMnZxQ1VDCkF3RUFBYU5HTUVRd0V3WURWUjBsQkF3d0NnWUlLd1lCQlFVSEF3SXdEQVlEVlIwVEFRSC9CQUl3QURBZkJnTlYKSFNNRUdEQVdnQlRPbnIwcys3VENKbkltcE8wNWhTMnBwY0poc0RBTkJna3Foa2lHOXcwQkFRc0ZBQU9DQVFFQQpwZnNmaHR0dXBYTUYyK0JVNDZwZGlNVzhqSmxEVE5MK2ZIbStTNFJlN2lqRmNZN0pGQ3VYQWNEczNQOUZJL05qCnhxaXRWaE1nL1g2NCtGN3JhYnVkN2Ftdm5yL2E0WE1JWlBCc1J5RzIrNzRJZ1JUK2ZLZ1RnalMyNHNZaUVyOVQKeHhHOEIxRGh0VHMrcVNhL1I2cml5dnU0aXdVS0o0SlR4ZGRYSENTZWJBWFRGTjg1UmkvMFBBNjd3blFKS3ZOZgpzWlhvZVhBMDVXdXhmN1VCQXMzcmtrbDVFSGVlaGlybXhSdGFLd25RRTlRc2pOa3RBc050NjRYc1ZUSXdYRHlhCnRRMTJNcHltem92RDhwTDJoYnZzeDRHdSsxQ0VWWUpCZ2txazU3ZUMwaGQxTjR0YWU2YVgrOFZZTDhnNUprRWQKOFA3SWxBWnZ2QjFWV3Z5a2JqbWN6dz09Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"
		ctx = context.Background()
	})

	JustBeforeEach(func() {
		identity, err = certInspector.WhoAmI(ctx, certPEMBase64)
	})

	It("extracts the common name from a x509 cert", func() {
		Expect(err).NotTo(HaveOccurred())
		Expect(identity.Name).To(Equal("cert-user-0a299b"))
		Expect(identity.Kind).To(Equal(rbacv1.UserKind))
	})

	When("the cert is invalid base64", func() {
		BeforeEach(func() {
			certPEMBase64 = "$*&^^%%"
		})

		It("returns an error", func() {
			Expect(err).To(MatchError("failed to base64 decode cert"))
		})
	})

	When("the cert is not PEM encoded", func() {
		BeforeEach(func() {
			certPEMBase64 = "Zm9vCg=="
		})

		It("returns an error", func() {
			Expect(err).To(MatchError("failed to decode PEM"))
		})
	})

	When("the cert contains multiple PEM blocks", func() {
		BeforeEach(func() {
			certPEMBase64 += getBadPEMBlock()
		})

		It("uses the first", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(identity.Name).To(Equal("cert-user-0a299b"))
		})
	})

	When("the cert cannot be parsed", func() {
		BeforeEach(func() {
			certPEMBase64 = getBadPEMBlock()
		})

		It("returns an error", func() {
			Expect(err).To(MatchError(ContainSubstring("failed to parse certificate")))
		})
	})
})

func getBadPEMBlock() string {
	certBytes := []byte("hello")
	pemBlock := pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	}
	certPEM := pem.EncodeToMemory(&pemBlock)
	return base64.StdEncoding.EncodeToString(certPEM)
}
