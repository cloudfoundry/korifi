package config_test

import (
	"os"
	"path/filepath"
	"time"

	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	"go.uber.org/zap/zapcore"
	"go.yaml.in/yaml/v3"
)

var _ = Describe("LoadFromPath", func() {
	var (
		configPath string
		retConfig  *config.ControllerConfig
		retErr     error
		cfg        map[string]any
	)

	BeforeEach(func() {
		// Setup filesystem
		var err error
		configPath, err = os.MkdirTemp("", "config")
		Expect(err).NotTo(HaveOccurred())

		cfg = map[string]any{
			"cfProcessDefaults": map[string]any{
				"memoryMB":    1024,
				"diskQuotaMB": 512,
				"timeout":     30,
			},
			"cfStagingResources": map[string]any{
				"buildCacheMB": 1024,
				"diskMB":       512,
				"memoryMB":     2048,
			},
			"cfRootNamespace":                  "rootNamespace",
			"containerRegistrySecretNames":     []string{"packageRegistrySecretName"},
			"taskTTL":                          "5h",
			"jobTTL":                           "1m",
			"builderReadinessTimeout":          "2s",
			"builderName":                      "buildReconciler",
			"runnerName":                       "statefulset-runner",
			"namespaceLabels":                  map[string]any{},
			"extraVCAPApplicationValues":       map[string]any{},
			"logLevel":                         "debug",
			"spaceFinalizerAppDeletionTimeout": 42,
			"networking": map[string]any{
				"gatewayName":      "gw-name",
				"gatewayNamespace": "gw-ns",
			},
			"experimentalManagedServicesEnabled": true,
			"trustInsecureServiceBrokers":        true,
			"includeKpackImageBuilder":           true,
			"includeJobTaskRunner":               true,
			"includeStatefulsetRunner":           true,
			"maxRetainedPackagesPerApp":          1,
			"maxRetainedBuildsPerApp":            2,
			"clusterBuilderName":                 "bldrName",
			"builderServiceAccount":              "bldrSvcAcc",
			"containerRepositoryPrefix":          "repoPrefix",
			"containerRegistryType":              "regisryType",
			"disableRouteController":             true,
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
		Expect(*retConfig).To(Equal(config.ControllerConfig{
			CFProcessDefaults: config.CFProcessDefaults{
				MemoryMB:    1024,
				DiskQuotaMB: 512,
				Timeout:     tools.PtrTo(int32(30)),
			},
			CFStagingResources: config.CFStagingResources{
				BuildCacheMB: 1024,
				DiskMB:       512,
				MemoryMB:     2048,
			},
			CFRootNamespace:                  "rootNamespace",
			ContainerRegistrySecretNames:     []string{"packageRegistrySecretName"},
			TaskTTL:                          5 * time.Hour,
			BuilderName:                      "buildReconciler",
			RunnerName:                       "statefulset-runner",
			NamespaceLabels:                  map[string]string{},
			ExtraVCAPApplicationValues:       map[string]any{},
			JobTTL:                           1 * time.Minute,
			BuilderReadinessTimeout:          2 * time.Second,
			LogLevel:                         zapcore.DebugLevel,
			SpaceFinalizerAppDeletionTimeout: tools.PtrTo(int32(42)),
			Networking: config.Networking{
				GatewayName:      "gw-name",
				GatewayNamespace: "gw-ns",
			},
			ExperimentalManagedServicesEnabled: true,
			TrustInsecureServiceBrokers:        true,
			IncludeKpackImageBuilder:           true,
			IncludeJobTaskRunner:               true,
			IncludeStatefulsetRunner:           true,
			MaxRetainedPackagesPerApp:          1,
			MaxRetainedBuildsPerApp:            2,
			ClusterBuilderName:                 "bldrName",
			BuilderServiceAccount:              "bldrSvcAcc",
			ContainerRepositoryPrefix:          "repoPrefix",
			ContainerRegistryType:              "regisryType",
			DisableRouteController:             true,
		}))
	})

	When("the CFProcess default timeout is not set", func() {
		BeforeEach(func() {
			cfg["cfProcessDefaults"] = map[string]any{}
		})

		It("uses the default", func() {
			Expect(retConfig.CFProcessDefaults.Timeout).To(gstruct.PointTo(BeEquivalentTo(60)))
		})
	})

	When("the space finalizer app deletion timeout is not set", func() {
		BeforeEach(func() {
			cfg["spaceFinalizerAppDeletionTimeout"] = nil
		})

		It("uses the default", func() {
			Expect(retConfig.SpaceFinalizerAppDeletionTimeout).To(gstruct.PointTo(BeEquivalentTo(60)))
		})
	})
})
