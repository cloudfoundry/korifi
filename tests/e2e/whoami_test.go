package e2e_test

import (
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"time"

	"code.cloudfoundry.org/korifi/tests/helpers"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"
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
		client   *helpers.CorrelatedRestyClient
		httpResp *resty.Response
		result   identityResource
	)

	JustBeforeEach(func() {
		var err error
		httpResp, err = client.R().
			SetResult(&result).
			Get("/whoami")
		Expect(err).NotTo(HaveOccurred())
	})

	When("authenticating with a Bearer token", func() {
		var svcAcctName string

		BeforeEach(func() {
			svcAcctName = uuid.NewString()
			serviceAccountToken := serviceAccountFactory.CreateServiceAccount(svcAcctName)
			client = makeTokenClient(serviceAccountToken)
		})

		AfterEach(func() {
			serviceAccountFactory.DeleteServiceAccount(svcAcctName)
		})

		It("returns the user identity", func() {
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Name).To(Equal(serviceAccountFactory.FullyQualifiedName(svcAcctName)))
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
		var userName string

		BeforeEach(func() {
			userName = uuid.NewString()
			client = makeCertClientForUserName(userName, time.Hour)
		})

		It("returns the user identity", func() {
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(result.Name).To(Equal(userName))
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
				client = client.SetAuthToken(base64.StdEncoding.EncodeToString(helpers.CreateSelfSignedCertificatePEM()))
			})

			It("returns an unauthorized error", func() {
				Expect(httpResp).To(HaveRestyStatusCode(http.StatusUnauthorized))
			})
		})
	})

	When("no Authorization header is available in the request", func() {
		BeforeEach(func() {
			client = helpers.NewCorrelatedRestyClient(apiServerRoot, getCorrelationId).SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
		})
		It("returns unauthorized error", func() {
			Expect(httpResp).To(HaveRestyStatusCode(http.StatusUnauthorized))
		})
	})
})
