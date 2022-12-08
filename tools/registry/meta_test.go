package registry_test

import (
	"code.cloudfoundry.org/korifi/tools/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ContainerRegistryMeta", func() {
	var containerRegistryMeta *registry.ContainerRegistryMeta

	BeforeEach(func() {
		containerRegistryMeta = registry.NewContainerRegistryMeta("my-repo-prefix.foo/bar", "plus-some/more-")
	})

	It("returns package repo name", func() {
		Expect(containerRegistryMeta.PackageRepoName("my-app")).To(Equal("plus-some/more-my-app-packages"))
	})

	It("returns package image ref", func() {
		Expect(containerRegistryMeta.PackageImageRef("my-app")).To(Equal("my-repo-prefix.foo/bar/plus-some/more-my-app-packages"))
	})

	It("returns droplet repo name", func() {
		Expect(containerRegistryMeta.DropletRepoName("my-app")).To(Equal("plus-some/more-my-app-droplets"))
	})

	It("returns package image ref", func() {
		Expect(containerRegistryMeta.DropletImageRef("my-app")).To(Equal("my-repo-prefix.foo/bar/plus-some/more-my-app-droplets"))
	})
})
