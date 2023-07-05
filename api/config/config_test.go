package config_test

import (
	"os"

	"go.uber.org/zap/zapcore"

	"code.cloudfoundry.org/korifi/api/config"
	"code.cloudfoundry.org/korifi/tools/registry"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
)

var _ = Describe("Config", func() {
	var (
		configMap map[string]interface{}
		cfg       *config.APIConfig
		loadErr   error
		cfgDir    string
	)

	BeforeEach(func() {
		var err error

		cfgDir, err = os.MkdirTemp("", "")
		Expect(err).NotTo(HaveOccurred())

		configMap = map[string]interface{}{
			"internalPort":      1,
			"idleTimeout":       2,
			"readTimeout":       3,
			"readHeaderTimeout": 4,
			"writeTimeout":      5,

			"externalFQDN": "api.foo",

			"rootNamespace":                            "root-ns",
			"builderName":                              "my-builder",
			"containerRepositoryPrefix":                "container.registry/my-prefix",
			"packageRegistrySecretName":                "package-registry-secret",
			"defaultDomainName":                        "default.domain",
			"userCertificateExpirationWarningDuration": "10s",
			"defaultLifecycleConfig": config.DefaultLifecycleConfig{
				Type:            "lc-type",
				Stack:           "lc-stack",
				StagingMemoryMB: 10,
				StagingDiskMB:   20,
			},
		}
	})

	JustBeforeEach(func() {
		cfgFile, err := os.CreateTemp(cfgDir, "")
		Expect(err).NotTo(HaveOccurred())

		configBytes, err := yaml.Marshal(configMap)
		Expect(err).NotTo(HaveOccurred())

		_, err = cfgFile.Write(configBytes)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfgFile.Close()).To(Succeed())
		cfg, loadErr = config.LoadFromPath(cfgDir)
	})

	AfterEach(func() {
		Expect(os.RemoveAll(cfgDir)).To(Succeed())
	})

	It("populates the config", func() {
		Expect(loadErr).NotTo(HaveOccurred())
		Expect(cfg.InternalPort).To(Equal(1))
		Expect(cfg.IdleTimeout).To(Equal(2))
		Expect(cfg.ReadTimeout).To(Equal(3))
		Expect(cfg.ReadHeaderTimeout).To(Equal(4))
		Expect(cfg.WriteTimeout).To(Equal(5))
		Expect(cfg.ExternalFQDN).To(Equal("api.foo"))
		Expect(cfg.ServerURL).To(Equal("https://api.foo"))
		Expect(cfg.RootNamespace).To(Equal("root-ns"))
		Expect(cfg.BuilderName).To(Equal("my-builder"))
		Expect(cfg.ContainerRepositoryPrefix).To(Equal("container.registry/my-prefix"))
		Expect(cfg.PackageRegistrySecretName).To(Equal("package-registry-secret"))
		Expect(cfg.DefaultDomainName).To(Equal("default.domain"))
		Expect(cfg.UserCertificateExpirationWarningDuration).To(Equal("10s"))
		Expect(cfg.DefaultLifecycleConfig).To(Equal(config.DefaultLifecycleConfig{
			Type:            "lc-type",
			Stack:           "lc-stack",
			StagingMemoryMB: 10,
			StagingDiskMB:   20,
		}))
		Expect(cfg.ContainerRegistryType).To(BeEmpty())
	})

	When("the FQDN is not specified", func() {
		BeforeEach(func() {
			delete(configMap, "externalFQDN")
		})

		It("returns an error", func() {
			Expect(loadErr).To(MatchError("ExternalFQDN not specified"))
		})
	})

	When("the container registry type is ECR", func() {
		BeforeEach(func() {
			configMap["containerRegistryType"] = registry.ECRContainerRegistryType
		})

		It("sets it in the config", func() {
			Expect(cfg.ContainerRegistryType).To(Equal(registry.ECRContainerRegistryType))
		})
	})

	When("the auth proxy is configured", func() {
		BeforeEach(func() {
			configMap["authProxyHost"] = "my-auth-proxy"
			configMap["authProxyCACert"] = "my-auth-proxy-ca-cert"
		})

		It("succeeds", func() {
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(cfg.AuthProxyHost).To(Equal("my-auth-proxy"))
			Expect(cfg.AuthProxyCACert).To(Equal("my-auth-proxy-ca-cert"))
		})

		When("the auth proxy CA cert is not set", func() {
			BeforeEach(func() {
				delete(configMap, "authProxyCACert")
			})

			It("returns an error", func() {
				Expect(loadErr).To(MatchError("AuthProxyHost requires a value for AuthProxyCACert"))
			})
		})

		When("the auth proxy is not set", func() {
			BeforeEach(func() {
				delete(configMap, "authProxyHost")
			})

			It("returns an error", func() {
				Expect(loadErr).To(MatchError("AuthProxyCACert requires a value for AuthProxyHost"))
			})
		})
	})

	When("the log level is configured", func() {
		BeforeEach(func() {
			configMap["logLevel"] = "debug"
		})

		It("succeeds", func() {
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(cfg.LogLevel).To(Equal(zapcore.DebugLevel))
		})

		When("the log level is not set", func() {
			BeforeEach(func() {
				delete(configMap, "logLevel")
			})

			It("uses the default", func() {
				Expect(loadErr).NotTo(HaveOccurred())
				Expect(cfg.LogLevel).To(Equal(zapcore.InfoLevel))
			})
		})
	})

	When("the UserCertificateExpirationWarningDuration is invalid", func() {
		BeforeEach(func() {
			configMap["userCertificateExpirationWarningDuration"] = "invalid-duration"
		})

		It("returns an error", func() {
			Expect(loadErr).To(MatchError(ContainSubstring("invalid duration format")))
		})
	})

	When("the builder is not specified", func() {
		BeforeEach(func() {
			delete(configMap, "builderName")
		})

		It("returns an error", func() {
			Expect(loadErr).To(MatchError("BuilderName must have a value"))
		})
	})

	When("external port is specified", func() {
		BeforeEach(func() {
			configMap["externalPort"] = 1234
		})

		It("populates the server URL", func() {
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(cfg.ServerURL).To(Equal("https://api.foo:1234"))
		})
	})
})
