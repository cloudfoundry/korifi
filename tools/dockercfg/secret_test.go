package dockercfg_test

import (
	"encoding/base64"

	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools/dockercfg"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("DockerConfigSecret", func() {
	var (
		repos  []dockercfg.DockerServerConfig
		secret *corev1.Secret
	)

	BeforeEach(func() {
		repos = []dockercfg.DockerServerConfig{{
			Server:   "myrepo",
			Username: "bob",
			Password: "password",
		}}
	})

	JustBeforeEach(func() {
		var err error
		secret, err = dockercfg.CreateDockerConfigSecret("secret-ns", "secret-name", repos...)
		Expect(err).NotTo(HaveOccurred())
	})

	It("creates a valid docker config secret", func() {
		Expect(secret.Namespace).To(Equal("secret-ns"))
		Expect(secret.Name).To(Equal("secret-name"))
		Expect(secret.Type).To(Equal(corev1.SecretTypeDockerConfigJson))

		Expect(secret.Data).To(HaveKeyWithValue(
			corev1.DockerConfigJsonKey,
			MatchJSONPath("$.auths.myrepo.auth", base64.StdEncoding.EncodeToString([]byte("bob:password"))),
		))
	})
	When("the server is not set", func() {
		BeforeEach(func() {
			repos[0].Server = ""
		})

		It("sets the server to https://index.docker.io/v1/", func() {
			Expect(secret.Data).To(HaveKeyWithValue(
				corev1.DockerConfigJsonKey,
				MatchJSONPath(`$.auths["https://index.docker.io/v1/"].auth`, base64.StdEncoding.EncodeToString([]byte("bob:password"))),
			))
		})
	})

	When("the server is index.docker.io", func() {
		BeforeEach(func() {
			repos[0].Server = "index.docker.io"
		})

		It("sets the server to https://index.docker.io/v1/", func() {
			Expect(secret.Data).To(HaveKeyWithValue(
				corev1.DockerConfigJsonKey,
				MatchJSONPath(`$.auths["https://index.docker.io/v1/"].auth`, base64.StdEncoding.EncodeToString([]byte("bob:password"))),
			))
		})
	})

	When("multiple registries are passed", func() {
		BeforeEach(func() {
			repos = append(repos, dockercfg.DockerServerConfig{Server: "myotherserver"})
		})

		It("adds entries for all servers", func() {
			Expect(secret.Data).To(HaveKeyWithValue(
				corev1.DockerConfigJsonKey,
				SatisfyAll(
					MatchJSONPath("$.auths.myrepo.auth", Not(BeEmpty())),
					MatchJSONPath("$.auths.myotherserver.auth", Not(BeEmpty())),
				)),
			)
		})
	})
})
