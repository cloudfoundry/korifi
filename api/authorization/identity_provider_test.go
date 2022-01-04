package authorization_test

import (
	"context"
	"errors"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization/fake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("IdentityProvider", func() {
	var (
		authInfo       authorization.Info
		tokenInspector *fake.TokenIdentityInspector
		certInspector  *fake.CertIdentityInspector
		idProvider     *authorization.IdentityProvider
		aliceId, id    authorization.Identity
		err            error
	)

	JustBeforeEach(func() {
		id, err = idProvider.GetIdentity(context.Background(), authInfo)
	})

	BeforeEach(func() {
		tokenInspector = new(fake.TokenIdentityInspector)
		certInspector = new(fake.CertIdentityInspector)
		idProvider = authorization.NewIdentityProvider(tokenInspector, certInspector)
		aliceId = authorization.Identity{Kind: rbacv1.UserKind, Name: "alice"}
		authInfo = authorization.Info{}
	})

	When("the authorization.Info contains a token", func() {
		BeforeEach(func() {
			authInfo.Token = "a-token"
			tokenInspector.WhoAmIReturns(aliceId, nil)
		})

		It("succeeds", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		It("gets the identity from the token inspector", func() {
			Expect(tokenInspector.WhoAmICallCount()).To(Equal(1))
			_, actualToken := tokenInspector.WhoAmIArgsForCall(0)
			Expect(actualToken).To(Equal("a-token"))
		})

		It("gets the identity associated with the given header", func() {
			Expect(id).To(Equal(aliceId))
		})

		When("token inspector fails", func() {
			BeforeEach(func() {
				tokenInspector.WhoAmIReturns(authorization.Identity{}, errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("boom")))
			})
		})
	})

	When("the authorization.Info contains a client cert", func() {
		BeforeEach(func() {
			authInfo.CertData = []byte("a-cert")
			certInspector.WhoAmIReturns(aliceId, nil)
		})

		It("succeeds", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		It("gets the identity from the cert inspector", func() {
			Expect(certInspector.WhoAmICallCount()).To(Equal(1))
			_, actualCert := certInspector.WhoAmIArgsForCall(0)
			Expect(actualCert).To(BeEquivalentTo("a-cert"))
		})

		It("gets the identity associated with the given header", func() {
			Expect(id).To(Equal(aliceId))
		})

		When("cert inspector fails", func() {
			BeforeEach(func() {
				certInspector.WhoAmIReturns(authorization.Identity{}, errors.New("boom"))
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("boom")))
			})
		})
	})

	When("the authorization.Info is empty", func() {
		It("fails", func() {
			Expect(err).To(HaveOccurred())
		})
	})
})
