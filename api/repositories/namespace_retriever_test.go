package repositories_test

import (
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"k8s.io/client-go/dynamic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("NamespaceRetriever", func() {
	var (
		resourceType       string
		appGUID            string
		orgGUID            string
		spaceGUID          string
		retNS              string
		retErr             error
		namespaceRetriever repositories.NamespaceRetriever
	)

	BeforeEach(func() {
		dynamicClient, err := dynamic.NewForConfig(testEnv.Config)
		Expect(err).NotTo(HaveOccurred())
		namespaceRetriever = repositories.NewNamespaceRetriever(dynamicClient)

		resourceType = repositories.AppResourceType
		appGUID = prefixedGUID("app")
		org := createOrgWithCleanup(ctx, prefixedGUID("org"))
		orgGUID = org.Name
		space := createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))
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
			space2 := createSpaceWithCleanup(ctx, orgGUID, prefixedGUID("space2"))
			_ = createAppCR(ctx, k8sClient, "app2", appGUID, space2.Name, "STOPPED")
		})

		It("returns a duplicate error", func() {
			Expect(retErr).To(MatchError(ContainSubstring("duplicate records exist")))
		})
	})
})
