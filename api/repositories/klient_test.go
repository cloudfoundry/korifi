package repositories_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Klient", func() {
	Describe("ListOptions", func() {
		Describe("AsClientListOptions", func() {
			var (
				listOptions *repositories.ListOptions
				clientOpts  *client.ListOptions
			)

			BeforeEach(func() {
				requrements, err := labels.ParseToRequirements("foo=bar")
				Expect(err).NotTo(HaveOccurred())

				listOptions = &repositories.ListOptions{
					Namespace:     "some-ns",
					FieldSelector: fields.SelectorFromSet(fields.Set{"bar": "baz"}),
					Requrements:   requrements,
				}
			})

			JustBeforeEach(func() {
				clientOpts = listOptions.AsClientListOptions()
			})

			It("returns the correct client list options", func() {
				labelSelector, err := labels.Parse("foo=bar")
				Expect(err).NotTo(HaveOccurred())

				Expect(clientOpts).To(Equal(&client.ListOptions{
					LabelSelector: labelSelector,
					FieldSelector: fields.SelectorFromSet(fields.Set{"bar": "baz"}),
					Namespace:     "some-ns",
				}))
			})
		})

		Describe("ApplyToList", func() {
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

			Describe("WithLabelExists", func() {
				BeforeEach(func() {
					option = repositories.WithLabelExists("foo")
				})

				It("adds the label selector requirements to the list options", func() {
					expectedReq, err := labels.NewRequirement("foo", selection.Exists, nil)
					Expect(err).NotTo(HaveOccurred())

					Expect(listOptions.Requrements).To(ConsistOf(*expectedReq))
				})

				When("the key is invalid", func() {
					BeforeEach(func() {
						option = repositories.WithLabelExists("~")
					})

					It("returns an error", func() {
						Expect(applyToListErr).To(MatchError(ContainSubstring("invalid label selector")))
					})
				})
			})

			Describe("SortBy", func() {
				BeforeEach(func() {
					option = repositories.SortBy("foo", true)
				})

				It("adds the sort by field to the list options", func() {
					Expect(applyToListErr).NotTo(HaveOccurred())
					Expect(listOptions.Sort).To(PointTo(Equal(repositories.SortOpt{
						By:   "foo",
						Desc: true,
					})))
				})
			})

			Describe("NoopListOption", func() {
				BeforeEach(func() {
					option = repositories.NoopListOption{}
				})

				It("does not modify the list options", func() {
					Expect(applyToListErr).NotTo(HaveOccurred())
					Expect(listOptions).To(PointTo(BeZero()))
				})
			})

			Describe("ErroringListOption", func() {
				BeforeEach(func() {
					option = repositories.ErroringListOption("error message")
				})

				It("returns an error", func() {
					Expect(applyToListErr).To(MatchError(ContainSubstring("error message")))
				})
			})

			Describe("WithLabelStrictlyIn", func() {
				BeforeEach(func() {
					option = repositories.WithLabelStrictlyIn("foo", []string{"bar"})
				})

				It("adds the label selector requirements to the list options", func() {
					expectedReq, err := labels.NewRequirement("foo", selection.In, []string{"bar"})
					Expect(err).NotTo(HaveOccurred())

					Expect(listOptions.Requrements).To(ConsistOf(*expectedReq))
				})

				When("the values are empty", func() {
					BeforeEach(func() {
						option = repositories.WithLabelStrictlyIn("foo", []string{})
					})

					It("adds match nothing requirements to the list options", func() {
						expectedReqs, _ := k8s.MatchNotingSelector().Requirements()
						Expect(listOptions.Requrements).To(ConsistOf(expectedReqs))
					})
				})
			})
		})
	})
})
