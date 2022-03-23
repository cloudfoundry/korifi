package repositories_test

import (
	"context"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NamespaceRetriever", func() {
	var (
		ctx          context.Context
		resourceType string
		appGUID      string
		orgGUID      string
		spaceGUID    string
		retNS        string
		retErr       error
	)

	BeforeEach(func() {
		ctx = context.Background()

		resourceType = repositories.AppResourceType
		appGUID = prefixedGUID("app")
		org := createOrgAnchorAndNamespace(ctx, rootNamespace, prefixedGUID("org"))
		orgGUID = org.Name
		space := createSpaceAnchorAndNamespace(ctx, org.Name, prefixedGUID("space"))
		spaceGUID = space.Name
		_ = createAppCR(ctx, k8sClient, "app1", appGUID, space.Name, "STOPPED")
	})

	JustBeforeEach(func() {
		retNS, retErr = namespaceRetriever.NamespaceFor(ctx, appGUID, resourceType)
	})

	It("returns the namespace for a unique GUID", func() {
		Expect(retErr).NotTo(HaveOccurred())
		Expect(retNS).To(Equal(spaceGUID))
	})

	When("the resource is not namespaced", func() {
		BeforeEach(func() {
			resourceType = "unknown-resource-type"
		})

		It("returns a duplicate error", func() {
			Expect(retErr).To(MatchError(`resource type "unknown-resource-type" unknown`))
		})
	})

	When("the guid does not exist", func() {
		BeforeEach(func() {
			appGUID = "does-not-exist"
		})

		It("returns a not found error", func() {
			Expect(retErr).To(BeAssignableToTypeOf(apierrors.NotFoundError{}))
		})
	})

	When("there are duplicate guids", func() {
		BeforeEach(func() {
			space2 := createSpaceAnchorAndNamespace(ctx, orgGUID, prefixedGUID("space2"))
			_ = createAppCR(ctx, k8sClient, "app2", appGUID, space2.Name, "STOPPED")
		})

		It("returns a duplicate error", func() {
			Expect(retErr).To(MatchError(ContainSubstring("duplicate records exist")))
		})
	})
})
