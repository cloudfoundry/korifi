package registry_test

import (
	"code.cloudfoundry.org/korifi/tools/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ContainerRegistryMeta", func() {
	var containerRegistryMeta *registry.ContainerRegistryMeta

	BeforeEach(func() {
		containerRegistryMeta = registry.NewContainerRegistryMeta("my-repo-prefix.foo/bar/plus-some/more-")
	})

	It("returns the package repository path", func() {
		Expect(containerRegistryMeta.PackageRepoPath("my-app")).To(Equal("bar/plus-some/more-my-app-packages"))
	})

	It("returns the package repository name", func() {
		Expect(containerRegistryMeta.PackageRepoName("my-app")).To(Equal("my-repo-prefix.foo/bar/plus-some/more-my-app-packages"))
	})

	It("returns the droplet repository path", func() {
		Expect(containerRegistryMeta.DropletRepoPath("my-app")).To(Equal("bar/plus-some/more-my-app-droplets"))
	})

	It("returns the droplet repository name", func() {
		Expect(containerRegistryMeta.DropletRepoName("my-app")).To(Equal("my-repo-prefix.foo/bar/plus-some/more-my-app-droplets"))
	})

	When("the repository path is just a slash", func() {
		BeforeEach(func() {
			containerRegistryMeta = registry.NewContainerRegistryMeta("my-repo-prefix.foo/")
		})

		It("parses the prefix correctly", func() {
			Expect(containerRegistryMeta.DropletRepoPath("my-app")).To(Equal("my-app-droplets"))
			Expect(containerRegistryMeta.DropletRepoName("my-app")).To(Equal("my-repo-prefix.foo/my-app-droplets"))
		})
	})

	When("the repository path is empty", func() {
		BeforeEach(func() {
			containerRegistryMeta = registry.NewContainerRegistryMeta("my-repo-prefix.foo")
		})

		It("parses the prefix correctly", func() {
			Expect(containerRegistryMeta.DropletRepoPath("my-app")).To(Equal("my-app-droplets"))
			Expect(containerRegistryMeta.DropletRepoName("my-app")).To(Equal("my-repo-prefix.foo/my-app-droplets"))
		})
	})
})
