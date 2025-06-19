package descriptors_test

import (
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors/fake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ = DescribeTable("Pluralizer",
	func(kind string, expectedPlural string, matchErr types.GomegaMatcher) {
		plural, err := pluralizer.Pluralize(schema.GroupVersionKind{
			Group:   "korifi.cloudfoundry.org",
			Version: "v1alpha1",
			Kind:    kind,
		})
		Expect(err).To(matchErr)
		Expect(plural).To(Equal(expectedPlural))
	},
	Entry("CFApp", "CFApp", "cfapps", Not(HaveOccurred())),
	Entry("CFBuild", "CFBuild", "cfbuilds", Not(HaveOccurred())),
	Entry("CFDomain", "CFDomain", "cfdomains", Not(HaveOccurred())),
	Entry("CFOrg", "CFOrg", "cforgs", Not(HaveOccurred())),
	Entry("CFPackage", "CFPackage", "cfpackages", Not(HaveOccurred())),
	Entry("CFProcess", "CFProcess", "cfprocesses", Not(HaveOccurred())),
	Entry("CFRoute", "CFRoute", "cfroutes", Not(HaveOccurred())),
	Entry("CFSecurityGroup", "CFSecurityGroup", "cfsecuritygroups", Not(HaveOccurred())),
	Entry("CFServiceBinding", "CFServiceBinding", "cfservicebindings", Not(HaveOccurred())),
	Entry("CFServiceBroker", "CFServiceBroker", "cfservicebrokers", Not(HaveOccurred())),
	Entry("CFServiceInstance", "CFServiceInstance", "cfserviceinstances", Not(HaveOccurred())),
	Entry("CFServiceOffering", "CFServiceOffering", "cfserviceofferings", Not(HaveOccurred())),
	Entry("CFServicePlan", "CFServicePlan", "cfserviceplans", Not(HaveOccurred())),
	Entry("CFSpace", "CFSpace", "cfspaces", Not(HaveOccurred())),
	Entry("CFTask", "CFTask", "cftasks", Not(HaveOccurred())),
	Entry("Nonexistent kind", "NoSuchKind", "", MatchError(ContainSubstring("not found in group/version"))),
)

var _ = Describe("Caching", func() {
	var (
		discoveryClient   *fake.DiscoveryInterface
		cachingPluralizer *descriptors.CachingPluralizer
		gvk               schema.GroupVersionKind
		plural            string
	)

	BeforeEach(func() {
		discoveryClient = new(fake.DiscoveryInterface)
		discoveryClient.ServerResourcesForGroupVersionReturns(&metav1.APIResourceList{
			APIResources: []metav1.APIResource{{
				Name: "foos",
				Kind: "Foo",
			}},
		}, nil)
		cachingPluralizer = descriptors.NewCachingPluralizer(discoveryClient)
		gvk = schema.GroupVersionKind{
			Group:   "foo.com",
			Version: "v1",
			Kind:    "Foo",
		}
	})

	JustBeforeEach(func() {
		var err error
		plural, err = cachingPluralizer.Pluralize(gvk)
		Expect(err).NotTo(HaveOccurred())
	})

	It("delegates to the discovery client", func() {
		Expect(discoveryClient.ServerResourcesForGroupVersionCallCount()).To(Equal(1))
		gv := discoveryClient.ServerResourcesForGroupVersionArgsForCall(0)
		Expect(gv).To(Equal("foo.com/v1"))
		Expect(plural).To(Equal("foos"))
	})

	When("we pluralize a kind for the second time", func() {
		BeforeEach(func() {
			var err error
			plural, err = cachingPluralizer.Pluralize(gvk)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the cached plural without calling the discovery client again", func() {
			Expect(discoveryClient.ServerResourcesForGroupVersionCallCount()).To(Equal(1))
			Expect(plural).To(Equal("foos"))
		})
	})
})
