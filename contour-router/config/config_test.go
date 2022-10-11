package config_test

import (
	"os"
	"path/filepath"

	"code.cloudfoundry.org/korifi/contour-router/config"
	"gopkg.in/yaml.v3"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LoadFromPath", func() {
	var (
		configPath string
		retConfig  *config.ContourRouterConfig
		retErr     error
		cfg        config.ContourRouterConfig
	)

	BeforeEach(func() {
		// Setup filesystem
		var err error
		configPath, err = os.MkdirTemp("", "config")
		Expect(err).NotTo(HaveOccurred())

		cfg = config.ContourRouterConfig{
			WorkloadsTLSSecretName:      "workloadsTLSSecretName",
			WorkloadsTLSSecretNamespace: "workloadsTLSSecretNamespace",
		}
	})

	AfterEach(func() {
		Expect(os.RemoveAll(configPath)).To(Succeed())
	})

	JustBeforeEach(func() {
		configYAML, err := yaml.Marshal(cfg)
		Expect(err).NotTo(HaveOccurred())

		Expect(os.WriteFile(filepath.Join(configPath, "file1"), configYAML, 0o644)).To(Succeed())
		retConfig, retErr = config.LoadFromPath(configPath)
	})

	It("loads the configuration from all the files in the given directory", func() {
		Expect(retErr).NotTo(HaveOccurred())
		Expect(*retConfig).To(Equal(config.ContourRouterConfig{
			WorkloadsTLSSecretName:      "workloadsTLSSecretName",
			WorkloadsTLSSecretNamespace: "workloadsTLSSecretNamespace",
		}))
	})
})
