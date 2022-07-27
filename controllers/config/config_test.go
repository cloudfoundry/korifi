package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"code.cloudfoundry.org/korifi/controllers/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LoadFromPath", func() {
	var (
		configPath string
		retConfig  *config.ControllerConfig
		retErr     error
	)

	BeforeEach(func() {
		// Setup filesystem
		var err error
		configPath, err = os.MkdirTemp("", "config")
		Expect(err).NotTo(HaveOccurred())

		config := config.ControllerConfig{
			CFProcessDefaults: config.CFProcessDefaults{
				MemoryMB:    1024,
				DiskQuotaMB: 512,
			},
			CFRootNamespace:             "rootNamespace",
			PackageRegistrySecretName:   "packageRegistrySecretName",
			TaskTTL:                     "taskTTL",
			WorkloadsTLSSecretName:      "workloadsTLSSecretName",
			WorkloadsTLSSecretNamespace: "workloadsTLSSecretNamespace",
			BuildReconciler:             "buildReconciler",
		}
		configYAML, err := yaml.Marshal(config)
		Expect(err).NotTo(HaveOccurred())

		err = os.WriteFile(filepath.Join(configPath, "file1"), configYAML, 0o644)
		Expect(err).NotTo(HaveOccurred())

		err = os.WriteFile(filepath.Join(configPath, "file2"), []byte(`buildReconciler: "newBuildReconciler"`), 0o644)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.RemoveAll(configPath)).To(Succeed())
	})

	JustBeforeEach(func() {
		retConfig, retErr = config.LoadFromPath(configPath)
	})

	It("loads the configuration from all the files in the given directory", func() {
		Expect(retErr).NotTo(HaveOccurred())
		Expect(*retConfig).To(Equal(config.ControllerConfig{
			CFProcessDefaults: config.CFProcessDefaults{
				MemoryMB:    1024,
				DiskQuotaMB: 512,
			},
			CFRootNamespace:             "rootNamespace",
			PackageRegistrySecretName:   "packageRegistrySecretName",
			TaskTTL:                     "taskTTL",
			WorkloadsTLSSecretName:      "workloadsTLSSecretName",
			WorkloadsTLSSecretNamespace: "workloadsTLSSecretNamespace",
			BuildReconciler:             "newBuildReconciler",
		}))
	})

	When("the path does not exist", func() {
		BeforeEach(func() {
			configPath = "notarealpath"
		})

		It("throws an error", func() {
			Expect(retErr).To(MatchError(fmt.Sprintf("error reading config dir %q: open %s: no such file or directory", configPath, configPath)))
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

	When("entering something parseable by time.ParseDuration", func() {
		BeforeEach(func() {
			taskTTLString = "12h30m5s20ns"
		})

		It("parses ok", func() {
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(taskTTL).To(Equal(12*time.Hour + 30*time.Minute + 5*time.Second + 20*time.Nanosecond))
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

	When("a simple day expression", func() {
		BeforeEach(func() {
			taskTTLString = "25d"
		})

		It("parses ok", func() {
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(taskTTL).To(Equal(25 * 24 * time.Hour))
		})
	})

	When("a compound day expression", func() {
		BeforeEach(func() {
			taskTTLString = "25d13h12m"
		})

		It("parses ok", func() {
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(taskTTL).To(Equal(25*24*time.Hour + 13*time.Hour + 12*time.Minute))
		})
	})

	When("a compound day erroneous expression", func() {
		BeforeEach(func() {
			taskTTLString = "25dlater"
		})

		It("parses ok", func() {
			Expect(parseErr).To(HaveOccurred())
		})
	})

	When("it contains more than 1 'd'", func() {
		BeforeEach(func() {
			taskTTLString = "1d2d"
		})

		It("returns an error", func() {
			Expect(parseErr).To(HaveOccurred())
		})
	})
})
