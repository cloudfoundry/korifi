package config_test

import (
	"os"
	"path/filepath"
	"time"

	"code.cloudfoundry.org/korifi/job-task-runner/config"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("JobTaskRunnerConfig", func() {
	var cfg *config.JobTaskRunnerConfig

	BeforeEach(func() {
		cfg = &config.JobTaskRunnerConfig{}
	})

	Describe("LoadFromPath", func() {
		var (
			configPath string
			loadErr    error
		)

		BeforeEach(func() {
			var err error
			configPath, err = os.MkdirTemp("", "config")
			Expect(err).NotTo(HaveOccurred())

			err = os.WriteFile(filepath.Join(configPath, "file1"), []byte("jobTTL: 5m"), 0o644)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(os.RemoveAll(configPath)).To(Succeed())
		})

		JustBeforeEach(func() {
			cfg, loadErr = config.LoadFromPath(configPath)
		})

		It("loads the configuration", func() {
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(*cfg).To(Equal(config.JobTaskRunnerConfig{
				JobTTL: "5m",
			}))
		})
	})

	Describe("ParseJobTTL", func() {
		var (
			jobTTL   time.Duration
			parseErr error
		)

		JustBeforeEach(func() {
			jobTTL, parseErr = cfg.ParseJobTTL()
		})

		It("return 30 days by default", func() {
			Expect(parseErr).NotTo(HaveOccurred())
			Expect(jobTTL).To(Equal(24 * time.Hour))
		})

		When("jobTTL is something parseable by tools.ParseDuration", func() {
			BeforeEach(func() {
				cfg.JobTTL = "5d12h"
			})

			It("parses ok", func() {
				Expect(parseErr).NotTo(HaveOccurred())
				Expect(jobTTL).To(Equal(5*24*time.Hour + 12*time.Hour))
			})
		})

		When("entering something that cannot be parsed", func() {
			BeforeEach(func() {
				cfg.JobTTL = "foreva"
			})

			It("returns an error", func() {
				Expect(parseErr).To(HaveOccurred())
			})
		})
	})
})
