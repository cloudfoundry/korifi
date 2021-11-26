package authorization_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
)

var _ = Describe("InfoParser", func() {
	var (
		authHeader string
		info       authorization.Info
		infoParser *authorization.InfoParser
		err        error
	)

	BeforeEach(func() {
		infoParser = authorization.NewInfoParser()
	})

	JustBeforeEach(func() {
		info, err = infoParser.Parse(authHeader)
	})

	When("the Authorization header contains a Bearer token", func() {
		BeforeEach(func() {
			authHeader = "Bearer token"
		})

		It("extracts the token from the header", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(info).To(Equal(authorization.Info{Token: "token"}))
		})

		When("the scheme is lowercase", func() {
			BeforeEach(func() {
				authHeader = "bearer token"
			})

			It("extracts the token from the header", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(info).To(Equal(authorization.Info{Token: "token"}))
			})
		})
	})

	When("the Authorization header contains a ClientCert", func() {
		BeforeEach(func() {
			authHeader = "ClientCert Zm9v"
		})

		It("extracts the cert and key data", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(info).To(Equal(authorization.Info{CertData: []byte("foo")}))
		})

		When("the scheme is lowercase", func() {
			BeforeEach(func() {
				authHeader = "clientcert Zm9v"
			})

			It("extracts the cert and key data", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(info).To(Equal(authorization.Info{CertData: []byte("foo")}))
			})
		})

		When("the cert data is not valid base64", func() {
			BeforeEach(func() {
				authHeader = "clientcert xxx"
			})

			It("returns an error", func() {
				Expect(err).To(BeAssignableToTypeOf(authorization.InvalidAuthError{}))
			})
		})
	})

	When("the authorization header uses an unsupported authentication scheme", func() {
		BeforeEach(func() {
			authHeader = "Scarer boo"
		})

		It("returns an error", func() {
			Expect(err).To(BeAssignableToTypeOf(authorization.InvalidAuthError{}))
		})
	})

	When("the authorization header is not recognized", func() {
		BeforeEach(func() {
			authHeader = "foo"
		})

		It("returns an error", func() {
			Expect(err).To(BeAssignableToTypeOf(authorization.InvalidAuthError{}))
		})
	})

	When("the authorization header is not set", func() {
		BeforeEach(func() {
			authHeader = ""
		})

		It("returns a NotAuthenticatedErr", func() {
			Expect(err).To(BeAssignableToTypeOf(authorization.NotAuthenticatedError{}))
		})
	})
})
