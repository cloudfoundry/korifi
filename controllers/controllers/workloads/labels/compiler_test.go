package labels_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/labels"
)

var _ = Describe("Labels", func() {
	var (
		compiler labels.Compiler
		override map[string]string
		output   map[string]string
	)

	BeforeEach(func() {
		override = nil
		compiler = labels.NewCompiler()
	})

	JustBeforeEach(func() {
		output = compiler.Compile(override)
	})

	It("will return empty if no defaults or override given", func() {
		Expect(output).To(BeEmpty())
	})

	When("default values are provided", func() {
		BeforeEach(func() {
			compiler = compiler.Defaults(map[string]string{
				"foo": "bar",
			})
		})

		It("puts the default in the output", func() {
			Expect(output).To(HaveKeyWithValue("foo", "bar"))
		})
	})

	When("default values are provided twice", func() {
		var oldCompiler labels.Compiler

		BeforeEach(func() {
			oldCompiler = compiler.Defaults(map[string]string{
				"foo":   "bar",
				"hello": "there",
			})
			compiler = oldCompiler.Defaults(map[string]string{
				"foo": "baz",
			})
		})

		It("puts the latest default in the output", func() {
			Expect(output).To(HaveKeyWithValue("foo", "baz"))
			Expect(output).To(HaveKeyWithValue("hello", "there"))
		})

		It("is immutable", func() {
			Expect(oldCompiler.Compile(nil)).To(HaveKeyWithValue("foo", "bar"))
			Expect(oldCompiler.Compile(nil)).To(HaveKeyWithValue("hello", "there"))
		})
	})

	When("a default value is overridden", func() {
		BeforeEach(func() {
			compiler = compiler.Defaults(map[string]string{
				"foo": "bar",
			})
			override = map[string]string{
				"foo": "baz",
			}
		})

		It("will use overridden value", func() {
			Expect(output).To(HaveKeyWithValue("foo", "baz"))
		})

		It("will not accidently store the override", func() {
			Expect(compiler.Compile(nil)).To(HaveKeyWithValue("foo", "bar"))
		})
	})
})
