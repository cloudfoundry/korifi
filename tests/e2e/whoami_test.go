package e2e_test

import (
	"crypto/tls"
	"encoding/base64"
	"net/http"

	"code.cloudfoundry.org/korifi/tests/helpers"
	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

type identityResource struct {
	resource
	Kind string `json:"kind"`
}

var _ = Describe("WhoAmI", func() {
	var (
		client   *resty.Client
		httpResp *resty.Response
		result   identityResource
	)

	BeforeEach(func() {
		client = resty.New().SetBaseURL(apiServerRoot).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	})

	JustBeforeEach(func() {
		var err error
		httpResp, err = client.R().
			SetResult(&result).
			Get("/whoami")
		Expect(err).NotTo(HaveOccurred())
	})

	When("authenticating with a Bearer token", func() {
		BeforeEach(func() {
			client = client.SetAuthToken(serviceAccountToken)
		})

		It("returns the user identity", func() {
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Name).To(Equal(serviceAccountName))
			Expect(result.Kind).To(Equal(rbacv1.ServiceAccountKind))
		})

		When("the token auth header is invalid", func() {
			BeforeEach(func() {
				client = client.SetAuthToken("not-valid")
			})

			It("returns an unauthorized error", func() {
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusUnauthorized))
			})
		})
	})

	When("authenticating with a client certificate", func() {
		BeforeEach(func() {
			client = client.SetAuthScheme("ClientCert").SetAuthToken(certPEM)
		})

		It("returns the user identity", func() {
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Name).To(Equal(certUserName))
			Expect(result.Kind).To(Equal(rbacv1.UserKind))
		})

		When("the cert auth header is invalid", func() {
			BeforeEach(func() {
				client = client.SetAuthToken("not-valid")
			})

			It("returns an unauthorized error", func() {
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusUnauthorized))
			})
		})

		When("the cert is unauthorized", func() {
			BeforeEach(func() {
				unauthorisedCertPEM := base64.StdEncoding.EncodeToString(helpers.CreateCertificatePEM())
				client = client.SetAuthToken(unauthorisedCertPEM)
			})

			It("returns an unauthorized error", func() {
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusUnauthorized))
			})
		})
	})

	When("no Authorization header is available in the request", func() {
		It("returns unauthorized error", func() {
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusUnauthorized))
		})
	})
})
