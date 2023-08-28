package dockercfg_test

import (
	"encoding/base64"

	"code.cloudfoundry.org/korifi/api/repositories/dockercfg"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("dockercfgjson", func() {
	var (
		server        string
		dockercfgjson string
	)

	BeforeEach(func() {
		server = "myrepo"
	})

	JustBeforeEach(func() {
		dockercfgjsonBytes, err := dockercfg.GenerateDockerCfgSecretData("bob", "password", server)
		Expect(err).NotTo(HaveOccurred())

		dockercfgjson = string(dockercfgjsonBytes)
	})

	It("generates a valid dockercfgjson", func() {
		Expect(dockercfgjson).To(
			MatchJSONPath("$.auths.myrepo.auth", base64.StdEncoding.EncodeToString([]byte("bob:password"))),
		)
	})

	When("the server is not set", func() {
		BeforeEach(func() {
			server = ""
		})

		It("sets the server to https://index.docker.io/v1/", func() {
			Expect(dockercfgjson).To(
				MatchJSONPath(`$.auths["https://index.docker.io/v1/"].auth`, base64.StdEncoding.EncodeToString([]byte("bob:password"))),
			)
		})
	})

	When("the server is index.docker.io", func() {
		BeforeEach(func() {
			server = "index.docker.io"
		})

		It("sets the server to https://index.docker.io/v1/", func() {
			Expect(dockercfgjson).To(
				MatchJSONPath(`$.auths["https://index.docker.io/v1/"].auth`, base64.StdEncoding.EncodeToString([]byte("bob:password"))),
			)
		})
	})
})
