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
	"gopkg.in/yaml.v3"
)

var _ = Describe("LoadFromPath", func() {
	var (
		configPath string
		retConfig  *config.ControllerConfig
		retErr     error
		cfg        config.ControllerConfig
	)

	BeforeEach(func() {
		// Setup filesystem
		var err error
		configPath, err = os.MkdirTemp("", "config")
		Expect(err).NotTo(HaveOccurred())

		cfg = config.ControllerConfig{
			CFProcessDefaults: config.CFProcessDefaults{
				MemoryMB:    1024,
				DiskQuotaMB: 512,
				Timeout:     tools.PtrTo(int64(30)),
			},
			CFRootNamespace:                  "rootNamespace",
			ContainerRegistrySecretName:      "packageRegistrySecretName",
			TaskTTL:                          "taskTTL",
			WorkloadsTLSSecretName:           "workloadsTLSSecretName",
			WorkloadsTLSSecretNamespace:      "workloadsTLSSecretNamespace",
			BuilderName:                      "buildReconciler",
			RunnerName:                       "statefulset-runner",
			JobTTL:                           "jobTTL",
			LogLevel:                         zapcore.DebugLevel,
			SpaceFinalizerAppDeletionTimeout: tools.PtrTo(int64(42)),
			BuildCacheMB:                     1024,
			DiskMB:                           512,
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
				Timeout:     tools.PtrTo(int64(30)),
			},
			CFRootNamespace:                  "rootNamespace",
			ContainerRegistrySecretName:      "packageRegistrySecretName",
			TaskTTL:                          "taskTTL",
			WorkloadsTLSSecretName:           "workloadsTLSSecretName",
			WorkloadsTLSSecretNamespace:      "workloadsTLSSecretNamespace",
			BuilderName:                      "buildReconciler",
			RunnerName:                       "statefulset-runner",
			NamespaceLabels:                  map[string]string{},
			ExtraVCAPApplicationValues:       map[string]any{},
			JobTTL:                           "jobTTL",
			LogLevel:                         zapcore.DebugLevel,
			SpaceFinalizerAppDeletionTimeout: tools.PtrTo(int64(42)),
			BuildCacheMB:                     1024,
			DiskMB:                           512,
		}))
	})

	When("the CFProcess default timeout is not set", func() {
		BeforeEach(func() {
			cfg.CFProcessDefaults.Timeout = nil
		})

		It("uses the default", func() {
			Expect(retConfig.CFProcessDefaults.Timeout).To(gstruct.PointTo(Equal(int64(60))))
		})
	})

	When("log level is not set", func() {
		BeforeEach(func() {
			cfg.LogLevel = 0
		})

		It("uses the default", func() {
			Expect(retConfig.LogLevel).To(Equal(zapcore.InfoLevel))
		})
	})

	When("the space finalizer app deletion timeout is not set", func() {
		BeforeEach(func() {
			cfg.SpaceFinalizerAppDeletionTimeout = nil
		})

		It("uses the default", func() {
			Expect(retConfig.SpaceFinalizerAppDeletionTimeout).To(gstruct.PointTo(Equal(int64(60))))
		})
	})

	When("the staging build cache size is not set", func() {
		BeforeEach(func() {
			cfg.BuildCacheMB = 0
		})

		It("uses the default", func() {
			Expect(retConfig.BuildCacheMB).To(Equal(2048))
		})
	})
})

var _ = Describe("ParseTaskTTL", func() {
	var (
		taskTTLString string
		taskTTL       time.Duration
		parseErr      error
	)

	BeforeEach(func() {
		taskTTLString = ""
	})

	JustBeforeEach(func() {
		cfg := config.ControllerConfig{
			TaskTTL: taskTTLString,
		}

		taskTTL, parseErr = cfg.ParseTaskTTL()
	})

	It("return 30 days by default", func() {
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(taskTTL).To(Equal(30 * 24 * time.Hour))
	})

	When("entering something parseable by tools.ParseDuration", func() {
		BeforeEach(func() {
			taskTTLString = "1d12h30m5s20ns"
		})

		It("parses ok", func() {
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(taskTTL).To(Equal(24*time.Hour + 12*time.Hour + 30*time.Minute + 5*time.Second + 20*time.Nanosecond))
		})
	})

	When("entering something that cannot be parsed", func() {
		BeforeEach(func() {
			taskTTLString = "foreva"
		})

		It("returns an error", func() {
			Expect(parseErr).To(HaveOccurred())
		})
	})
})

var _ = Describe("ParseJobTTL", func() {
	var (
		jobTTL    time.Duration
		parseErr  error
		jobTTLStr string
	)

	BeforeEach(func() {
		jobTTLStr = ""
	})

	JustBeforeEach(func() {
		cfg := config.ControllerConfig{
			JobTTL: jobTTLStr,
		}
		jobTTL, parseErr = cfg.ParseJobTTL()
	})

	It("return 30 days by default", func() {
		Expect(parseErr).NotTo(HaveOccurred())
		Expect(jobTTL).To(Equal(24 * time.Hour))
	})

	When("jobTTL is something parseable by tools.ParseDuration", func() {
		BeforeEach(func() {
			jobTTLStr = "5d12h"
		})

		It("parses ok", func() {
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(jobTTL).To(Equal(5*24*time.Hour + 12*time.Hour))
		})
	})

	When("entering something that cannot be parsed", func() {
		BeforeEach(func() {
			jobTTLStr = "foreva"
		})

		It("returns an error", func() {
			Expect(parseErr).To(HaveOccurred())
		})
	})
})
