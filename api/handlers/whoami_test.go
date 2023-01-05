package handlers_test

import (
	"errors"
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("WhoAmI", func() {
	var (
		apiHandler       *handlers.WhoAmI
		identityProvider *fake.IdentityProvider
	)

	BeforeEach(func() {
		identityProvider = new(fake.IdentityProvider)
		identityProvider.GetIdentityReturns(authorization.Identity{Name: "the-user", Kind: rbacv1.UserKind}, nil)
		ctx = authorization.NewContext(ctx, &authorization.Info{Token: "the-token"})
		apiHandler = handlers.NewWhoAmI(identityProvider, *serverURL)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/whoami", nil)
		Expect(err).NotTo(HaveOccurred())

		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("Who Am I", func() {
		It("returns 201 with appropriate success JSON", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(MatchJSON(`{
                "name": "the-user",
                "kind": "User"
            }`)))
		})

		It("calls the identity provider with the authorization.Info from the request context", func() {
			Expect(identityProvider.GetIdentityCallCount()).To(Equal(1))
			_, actualAuthInfo := identityProvider.GetIdentityArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authorization.Info{Token: "the-token"}))
		})

		When("the identity provider returns an error", func() {
			BeforeEach(func() {
				identityProvider.GetIdentityReturns(authorization.Identity{}, errors.New("boom"))
			})

			It("returns an unknown response", func() {
				expectUnknownError()
			})
		})
	})
})
