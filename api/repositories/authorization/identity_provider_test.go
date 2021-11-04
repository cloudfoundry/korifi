package authorization_test

import (
	"context"
	"errors"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization/fake"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("IdentityProvider", func() {
	var (
		authHeader     string
		tokenInspector *fake.IdentityInspector
		idProvider     *authorization.IdentityProvider
		aliceId, id    authorization.Identity
		err            error
	)

	BeforeEach(func() {
		aliceId = authorization.Identity{Kind: rbacv1.UserKind, Name: "alice"}
		authHeader = "Bearer token"
		tokenInspector = new(fake.IdentityInspector)
		idProvider = authorization.NewIdentityProvider(tokenInspector)
		tokenInspector.WhoAmIReturns(aliceId, nil)
	})

	JustBeforeEach(func() {
		id, err = idProvider.GetIdentity(context.Background(), authHeader)
	})

	It("succeeds", func() {
		Expect(err).NotTo(HaveOccurred())
	})

	It("gets the identity from the token inspector", func() {
		Expect(tokenInspector.WhoAmICallCount()).To(Equal(1))
		_, actualToken := tokenInspector.WhoAmIArgsForCall(0)
		Expect(actualToken).To(Equal("token"))
	})

	It("gets the identity associated with the given header", func() {
		Expect(id).To(Equal(aliceId))
	})

	When("the scheme is lowercase", func() {
		BeforeEach(func() {
			authHeader = "bearer token"
		})

		It("gets the identity from the token inspector", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(tokenInspector.WhoAmICallCount()).To(Equal(1))
			_, actualToken := tokenInspector.WhoAmIArgsForCall(0)
			Expect(actualToken).To(Equal("token"))
		})
	})

	When("token inspector fails", func() {
		BeforeEach(func() {
			tokenInspector.WhoAmIReturns(authorization.Identity{}, errors.New("boom"))
		})

		It("returns an error", func() {
			Expect(err).To(MatchError(ContainSubstring("boom")))
		})
	})

	When("the authorization header uses an unsupported authentication scheme", func() {
		BeforeEach(func() {
			authHeader = "Scarer boo"
		})

		It("returns an error", func() {
			Expect(err).To(MatchError(ContainSubstring("unsupported authentication scheme")))
		})
	})

	When("the authorization header is not recognized", func() {
		BeforeEach(func() {
			authHeader = "foo"
		})

		It("returns an error", func() {
			Expect(err).To(MatchError(ContainSubstring("failed to parse authorization header")))
		})
	})

	When("the authorization header is not set", func() {
		BeforeEach(func() {
			authHeader = ""
		})

		It("returns an UnauthorizedErr", func() {
			Expect(err).To(BeAssignableToTypeOf(authorization.UnauthorizedErr{}))
		})
	})
})
