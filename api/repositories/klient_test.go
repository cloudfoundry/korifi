package repositories_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

	"code.cloudfoundry.org/korifi/api/repositories"
)

var _ = Describe("Klient", func() {
	Describe("ListOptions", func() {
		var (
			option         repositories.ListOption
			listOptions    *repositories.ListOptions
			applyToListErr error
		)

		BeforeEach(func() {
			listOptions = &repositories.ListOptions{}
		})

		JustBeforeEach(func() {
			applyToListErr = option.ApplyToList(listOptions)
		})

		Describe("InNamespace", func() {
			BeforeEach(func() {
				option = repositories.InNamespace("in-ns")
			})

			It("adds the namespace to the list options", func() {
				Expect(applyToListErr).NotTo(HaveOccurred())
				Expect(listOptions.Namespace).To(Equal("in-ns"))
			})
		})

		Describe("MatchingFields", func() {
			BeforeEach(func() {
				option = repositories.MatchingFields{"field": "fieldValue"}
			})

			It("adds the matching fields to the list options", func() {
				expectedFieldSelector, err := fields.ParseSelector("field=fieldValue")
				Expect(err).NotTo(HaveOccurred())

				Expect(listOptions.FieldSelector).To(Equal(expectedFieldSelector))
			})
		})

		Describe("WithLabelSelector", func() {
			BeforeEach(func() {
				option = repositories.WithLabelSelector("foo==bar")
			})

			It("adds the label selector requirements to the list options", func() {
				expectedReq, err := labels.NewRequirement("foo", selection.DoubleEquals, []string{"bar"})
				Expect(err).NotTo(HaveOccurred())

				Expect(listOptions.Requrements).To(ConsistOf(*expectedReq))
			})

			When("the selector is invalid", func() {
				BeforeEach(func() {
					option = repositories.WithLabelSelector("~")
				})

				It("returns an error", func() {
					Expect(applyToListErr).To(MatchError(ContainSubstring("invalid label selector")))
				})
			})

			DescribeTable("valid label selectors",
				func(selector string) {
					option = repositories.WithLabelSelector(selector)
					Expect(option.ApplyToList(&repositories.ListOptions{})).To(Succeed())
				},
				Entry("key", "foo"),
				Entry("!key", "!foo"),
				Entry("key=value", "foo=FOO1"),
				Entry("key==value", "foo==FOO2"),
				Entry("key!=value", "foo!=FOO1"),
				Entry("key in (value1,value2)", "foo in (FOO1,FOO2)"),
				Entry("key notin (value1,value2)", "foo notin (FOO2)"),
			)
		})

		Describe("WithLabel", func() {
			BeforeEach(func() {
				option = repositories.WithLabel("foo", "bar")
			})

			It("adds the label selector requirements to the list options", func() {
				expectedReq, err := labels.NewRequirement("foo", selection.Equals, []string{"bar"})
				Expect(err).NotTo(HaveOccurred())

				Expect(listOptions.Requrements).To(ConsistOf(*expectedReq))
			})

			When("the key is invalid", func() {
				BeforeEach(func() {
					option = repositories.WithLabel("~", "bar")
				})

				It("returns an error", func() {
					Expect(applyToListErr).To(MatchError(ContainSubstring("invalid label selector")))
				})
			})
		})

		Describe("WithLabelIn", func() {
			BeforeEach(func() {
				option = repositories.WithLabelIn("foo", []string{"bar"})
			})

			It("adds the label selector requirements to the list options", func() {
				expectedReq, err := labels.NewRequirement("foo", selection.In, []string{"bar"})
				Expect(err).NotTo(HaveOccurred())

				Expect(listOptions.Requrements).To(ConsistOf(*expectedReq))
			})

			When("the values are empty", func() {
				BeforeEach(func() {
					option = repositories.WithLabelIn("foo", []string{})
				})

				It("does not add a requirement", func() {
					Expect(applyToListErr).NotTo(HaveOccurred())
					Expect(listOptions.Requrements).To(BeEmpty())
				})
			})

			When("the key is invalid", func() {
				BeforeEach(func() {
					option = repositories.WithLabelIn("~", []string{"bar"})
				})

				It("returns an error", func() {
					Expect(applyToListErr).To(MatchError(ContainSubstring("invalid label selector")))
				})
			})
		})
	})
})
