package tools_test

import (
	"os"
	"path/filepath"

	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type exampleConfig struct {
	Foo string `yaml:"foo"`
	Bar *int   `yaml:"bar"`
}

var _ = Describe("ConfigLoader", func() {
	var (
		conf    exampleConfig
		loadErr error
		dir     string
	)

	BeforeEach(func() {
		var err error
		dir, err = os.MkdirTemp("", "")
		Expect(err).NotTo(HaveOccurred())

		Expect(os.WriteFile(filepath.Join(dir, "file01"), []byte("foo: hello"), 0o644)).To(Succeed())

		conf = exampleConfig{}
	})

	AfterEach(func() {
		Expect(os.RemoveAll(dir)).To(Succeed())
	})

	JustBeforeEach(func() {
		loadErr = tools.LoadConfigInto(&conf, dir)
	})

	It("loads the file into the config", func() {
		Expect(loadErr).NotTo(HaveOccurred())
		Expect(conf).To(Equal(exampleConfig{Foo: "hello"}))
	})

	When("there are multiple files in the directory", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(filepath.Join(dir, "file02"), []byte("bar: 123"), 0o644)).To(Succeed())
		})

		It("loads all the files into the config", func() {
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(conf).To(Equal(exampleConfig{Foo: "hello", Bar: tools.PtrTo(123)}))
		})
	})

	When("the directory contains hidden files", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(filepath.Join(dir, ".file02"), []byte("bar: 123"), 0o644)).To(Succeed())
		})

		It("ignores them", func() {
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(conf).To(Equal(exampleConfig{Foo: "hello"}))
		})
	})

	When("the directory contains subdirectories", func() {
		BeforeEach(func() {
			Expect(os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dir, "subdir", "file02"), []byte("bar: 123"), 0o644)).To(Succeed())
		})

		It("ignores the files in the subdirectories", func() {
			Expect(loadErr).NotTo(HaveOccurred())
			Expect(conf).To(Equal(exampleConfig{Foo: "hello"}))
		})
	})

	When("the directory read fails", func() {
		BeforeEach(func() {
			dir = "i-do-not-exist"
		})

		It("returns an error", func() {
			Expect(loadErr).To(MatchError(ContainSubstring("error reading config dir")))
		})
	})

	When("the source file contains invalid yaml", func() {
		BeforeEach(func() {
			Expect(os.WriteFile(filepath.Join(dir, "file01"), []byte("hello"), 0o644)).To(Succeed())
		})

		It("returns an error", func() {
			Expect(loadErr).To(MatchError(ContainSubstring("failed decoding")))
		})
	})
})
