package util_test

import (
	"code.cloudfoundry.org/korifi/statefulset-runner/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parse", func() {
	Describe("AppIndex", func() {
		It("should index from pod name", func() {
			Expect(util.ParseAppIndex("some-name-1")).To(Equal(1))
		})

		Context("when the pod name does not contain dashes", func() {
			It("should return an error", func() {
				_, err := util.ParseAppIndex("somename1")

				Expect(err).To(HaveOccurred())
			})
		})

		Context("when the last part in pod name is not a number", func() {
			It("should return an error", func() {
				_, err := util.ParseAppIndex("somename-a")

				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("ImageRegistryHost", func() {
		It("returns the registry host when port is set", func() {
			imageURL := "my-secret-docker-registry.docker.io:5000/repo/the-mighty-image:not-latest"
			Expect(util.ParseImageRegistryHost(imageURL)).To(Equal("my-secret-docker-registry.docker.io"))
		})

		It("returns the registry host when port is not set", func() {
			imageURL := "my-secret-docker-registry.docker.io/repo/the-mighty-image:not-latest"
			Expect(util.ParseImageRegistryHost(imageURL)).To(Equal("my-secret-docker-registry.docker.io"))
		})

		It("should default to the docker hub with just 1 slash", func() {
			imageURL := "repo/the-mighty-image"
			Expect(util.ParseImageRegistryHost(imageURL)).To(Equal("index.docker.io/v1/"))
		})

		It("should default to the docker hub with just 1 slash and a late colon", func() {
			imageURL := "repo/the-mighty-image:not-latest"
			Expect(util.ParseImageRegistryHost(imageURL)).To(Equal("index.docker.io/v1/"))
		})

		It("should default to the docker hub with no slashes", func() {
			imageURL := "busybox"
			Expect(util.ParseImageRegistryHost(imageURL)).To(Equal("index.docker.io/v1/"))
		})
	})
})
