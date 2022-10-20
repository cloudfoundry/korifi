package handlers_test

import (
	"errors"
	"net/http"

	"code.cloudfoundry.org/korifi/api/authorization"
	apis "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"github.com/go-http-utils/headers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("WhoAmI", func() {
	const whoAmIBase = "/whoami"

	var (
		whoAmIHandler    *apis.WhoAmIHandler
		identityProvider *fake.IdentityProvider
		requestMethod    string
		requestPath      string
	)

	BeforeEach(func() {
		requestPath = whoAmIBase
		identityProvider = new(fake.IdentityProvider)
		identityProvider.GetIdentityReturns(authorization.Identity{
			Name:   "the-user",
			Groups: []string{"foo", "bar"},
			Kind:   rbacv1.UserKind,
		}, nil)
		ctx = authorization.NewContext(ctx, &authorization.Info{Token: "the-token"})
		whoAmIHandler = apis.NewWhoAmI(identityProvider, *serverURL)
		whoAmIHandler.RegisterRoutes(router)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestPath, nil)
		req.Header.Add(headers.Authorization, authHeader)
		Expect(err).NotTo(HaveOccurred())

		router.ServeHTTP(rr, req)
	})

	Describe("Who Am I", func() {
		It("returns 201 with appropriate success JSON", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(MatchJSON(`{
                "name": "the-user",
                "groups": ["foo", "bar"],
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
