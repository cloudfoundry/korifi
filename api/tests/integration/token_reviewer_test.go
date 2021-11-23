package integration_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	rbacv1 "k8s.io/api/rbac/v1"
)

var _ = Describe("TokenReviewer", func() {
	var (
		ctx                context.Context
		tokenReviewer      authorization.IdentityInspector
		token              string
		passErrConstraints types.GomegaMatcher
		id                 authorization.Identity
	)

	BeforeEach(func() {
		ctx = context.Background()
		tokenReviewer = authorization.NewTokenReviewer(k8sClient, k8sConfig)
		token = authProvider.GenerateJWTToken("alice")
		passErrConstraints = Succeed()
	})

	JustBeforeEach(func() {
		Eventually(func() error {
			var err error
			id, err = tokenReviewer.WhoAmI(ctx, token)
			return err
		}).Should(passErrConstraints)
	})

	It("extracts identity from a valid token", func() {
		Expect(id.Kind).To(Equal(rbacv1.UserKind))
		Expect(id.Name).To(Equal(oidcPrefix + "alice"))
	})

	It("configures a rest.Config for the identity", func() {
		Expect(id.Config.BearerToken).To(Equal(token))
		Expect(id.Config.CertData).To(BeEmpty())
		Expect(id.Config.KeyData).To(BeEmpty())
	})

	When("the token is issued for a serviceaccount", func() {
		BeforeEach(func() {
			restartEnvTest(authProvider.APIServerExtraArgs("system:serviceaccount:")...)
			token = authProvider.GenerateJWTToken(
				"my-serviceaccount",
				"system:serviceaccounts",
			)
			tokenReviewer = authorization.NewTokenReviewer(k8sClient, k8sConfig)
		})

		It("extracts the identity of the serviceaccount", func() {
			Expect(id.Kind).To(Equal(rbacv1.ServiceAccountKind))
			Expect(id.Name).To(Equal("my-serviceaccount"))
		})
	})

	When("the serviceaccount token is malformed", func() {
		BeforeEach(func() {
			restartEnvTest(authProvider.APIServerExtraArgs("incorrect-prefix:")...)
			token = authProvider.GenerateJWTToken(
				"my-serviceaccount",
				"system:serviceaccounts",
			)
			tokenReviewer = authorization.NewTokenReviewer(k8sClient, k8sConfig)
			passErrConstraints = MatchError(ContainSubstring("invalid serviceaccount name"))
		})

		It("returns an error", func() {
			Expect(id).To(Equal(authorization.Identity{}))
		})
	})

	When("an invalid token is passed", func() {
		BeforeEach(func() {
			token = "invalid"
			passErrConstraints = BeAssignableToTypeOf(authorization.InvalidAuthError{})
		})

		It("returns an error", func() {
			Expect(id).To(Equal(authorization.Identity{}))
		})
	})

	When("creating the token review fails", func() {
		var cancelCtx context.CancelFunc

		BeforeEach(func() {
			ctx, cancelCtx = context.WithDeadline(ctx, time.Now().Add(-time.Minute))
			passErrConstraints = MatchError(ContainSubstring("failed to create token review"))
		})

		AfterEach(func() {
			cancelCtx()
		})

		It("returns an error", func() {
			Expect(id).To(Equal(authorization.Identity{}))
		})
	})
})
