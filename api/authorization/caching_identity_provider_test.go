package authorization_test

import (
	"context"
	"errors"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/authorization/fake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/cache"
	"k8s.io/utils/clock/testing"
)

var _ = Describe("IdentityProvider", func() {
	var (
		authInfo      authorization.Info
		fakeProvider  *fake.IdentityProvider
		idProvider    *authorization.CachingIdentityProvider
		aliceId, id   authorization.Identity
		identityCache *cache.Expiring
		getErr        error
	)

	BeforeEach(func() {
		fakeProvider = new(fake.IdentityProvider)
		identityCache = cache.NewExpiringWithClock(testing.NewFakeClock(time.Now()))

		aliceId = authorization.Identity{Kind: rbacv1.UserKind, Name: "alice"}
		authInfo = authorization.Info{
			Token: "a-token",
		}
		fakeProvider.GetIdentityReturns(aliceId, nil)

		idProvider = authorization.NewCachingIdentityProvider(fakeProvider, identityCache)
	})

	JustBeforeEach(func() {
		id, getErr = idProvider.GetIdentity(context.Background(), authInfo)
	})

	It("succeeds", func() {
		Expect(getErr).NotTo(HaveOccurred())
	})

	It("gets the auth info from the real identity provider", func() {
		Expect(fakeProvider.GetIdentityCallCount()).To(Equal(1))
		_, actualAuthInfo := fakeProvider.GetIdentityArgsForCall(0)
		Expect(actualAuthInfo).To(Equal(authInfo))
	})

	When("the real identity provider fails", func() {
		BeforeEach(func() {
			fakeProvider.GetIdentityReturns(authorization.Identity{}, errors.New("boom"))
		})

		It("returns an error", func() {
			Expect(getErr).To(MatchError(ContainSubstring("boom")))
		})
	})

	When("the token is cached", func() {
		BeforeEach(func() {
			_, err := idProvider.GetIdentity(context.Background(), authInfo)
			Expect(err).NotTo(HaveOccurred())
		})

		It("does not call the token inspector", func() {
			Expect(fakeProvider.GetIdentityCallCount()).To(Equal(1))
		})

		It("returns the correct identity", func() {
			Expect(id).To(Equal(aliceId))
		})

		It("uses the hash of the auth info as a key", func() {
			Expect(identityCache.Len()).To(Equal(1))
			_, ok := identityCache.Get(authInfo.Hash())
			Expect(ok).To(BeTrue())
		})

		When("a different auth info is sent", func() {
			BeforeEach(func() {
				newAuthInfo := authorization.Info{
					Token: "new-token",
				}
				_, err := idProvider.GetIdentity(context.Background(), newAuthInfo)
				Expect(err).NotTo(HaveOccurred())
			})

			It("gets the identity both times", func() {
				Expect(fakeProvider.GetIdentityCallCount()).To(Equal(2))
			})
		})

		When("the cache has a non Identity value for the key", func() {
			BeforeEach(func() {
				identityCache.Set(authInfo.Hash(), 42, time.Minute)
			})

			It("returns an error", func() {
				Expect(getErr).To(MatchError(ContainSubstring("identity-provider cache: expected authorization.Identity{}, got int")))
			})
		})
	})
})
