package e2e_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/tests/helpers"
	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("WhoAmI", func() {
	It("returns the identity from the token authorization header", func() {
		identity, err := doWhoAmI(tokenAuthHeader)
		Expect(err).NotTo(HaveOccurred())
		Expect(identity.Name).To(Equal(serviceAccountName))
		Expect(identity.Kind).To(Equal(rbacv1.ServiceAccountKind))
	})

	It("returns the identity from the cert authorization header", func() {
		identity, err := doWhoAmI(certAuthHeader)
		Expect(err).NotTo(HaveOccurred())
		Expect(identity.Name).To(Equal(certUserName))
		Expect(identity.Kind).To(Equal(rbacv1.UserKind))
	})

	When("no Authorization header is available in the request", func() {
		It("returns unauthorized error", func() {
			resp, err := doWhoAmIRaw("")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(HaveHTTPStatus(http.StatusUnauthorized))
		})
	})

	When("the token auth header is invalid", func() {
		It("returns an unauthorized error", func() {
			resp, err := doWhoAmIRaw("Bearer not-a-valid-token")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(HaveHTTPStatus(http.StatusUnauthorized))
		})
	})

	When("the cert auth header is invalid", func() {
		It("returns an unauthorized error", func() {
			resp, err := doWhoAmIRaw("ClientCert not-a-valid-cert")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(HaveHTTPStatus(http.StatusUnauthorized))
		})
	})

	When("the cert auth header contains an unauthorized cert", func() {
		It("returns an unauthorized error", func() {
			resp, err := doWhoAmIRaw(helpers.CreateCertificateAuthHeader())
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(HaveHTTPStatus(http.StatusUnauthorized))
		})
	})
})

func doWhoAmIRaw(authHeaderValue string) (*http.Response, error) {
	whoamiURL := apiServerRoot + "/whoami"

	req, err := http.NewRequest(http.MethodGet, whoamiURL, nil)
	if err != nil {
		return nil, err
	}

	if authHeaderValue != "" {
		req.Header.Add(headers.Authorization, authHeaderValue)
	}

	return http.DefaultClient.Do(req)
}

func doWhoAmI(authHeaderValue string) (presenter.IdentityResponse, error) {
	resp, err := doWhoAmIRaw(authHeaderValue)
	if err != nil {
		return presenter.IdentityResponse{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return presenter.IdentityResponse{}, fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return presenter.IdentityResponse{}, err
	}

	identity := presenter.IdentityResponse{}
	err = json.Unmarshal(bodyBytes, &identity)
	if err != nil {
		return presenter.IdentityResponse{}, err
	}

	return identity, nil
}
