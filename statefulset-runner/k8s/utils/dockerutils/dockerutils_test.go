package dockerutils_test

import (
	"encoding/base64"
	"encoding/json"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/utils/dockerutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Docker Config", func() {
	var config *dockerutils.Config

	BeforeEach(func() {
		config = dockerutils.NewDockerConfig("host", "user", "pass")
	})

	It("Generates a valid docker config json", func() {
		configJSON, err := config.JSON()
		Expect(err).NotTo(HaveOccurred())

		config := make(map[string]map[string]map[string]string)
		Expect(json.Unmarshal([]byte(configJSON), &config)).To(Succeed())

		Expect(config).To(HaveKey("auths"))
		Expect(config["auths"]).To(HaveKey("host"))
		Expect(config["auths"]["host"]).To(HaveKeyWithValue("username", "user"))
		Expect(config["auths"]["host"]).To(HaveKeyWithValue("password", "pass"))

		auth := base64.StdEncoding.EncodeToString([]byte("user:pass"))
		Expect(config["auths"]["host"]).To(HaveKeyWithValue("auth", auth))
	})
})
