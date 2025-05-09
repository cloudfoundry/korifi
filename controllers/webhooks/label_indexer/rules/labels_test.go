package rules_test

import (
	"errors"

	"code.cloudfoundry.org/korifi/controllers/webhooks/label_indexer/rules"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Labels", func() {
	Describe("LabelRule", func() {
		var labelRule rules.LabelRule

		BeforeEach(func() {
			labelRule = rules.LabelRule{
				Label: "test-label",
				IndexingFunc: func(obj map[string]any) (*string, error) {
					return tools.PtrTo(obj["key"].(string)), nil
				},
			}
		})

		Describe("Apply", func() {
			var (
				labels   map[string]string
				applyErr error
			)

			JustBeforeEach(func() {
				labels, applyErr = labelRule.Apply(map[string]any{
					"key": "value",
				})
			})

			It("returns a map with labels", func() {
				Expect(applyErr).NotTo(HaveOccurred())
				Expect(labels).To(Equal(map[string]string{"test-label": "value"}))
			})

			When("the indexing func returns nil", func() {
				BeforeEach(func() {
					labelRule = rules.LabelRule{
						Label: "test-label",
						IndexingFunc: func(obj map[string]any) (*string, error) {
							return nil, nil
						},
					}
				})

				It("does not return such a label", func() {
					Expect(applyErr).NotTo(HaveOccurred())
					Expect(labels).To(BeEmpty())
				})
			})

			When("the indexing func errors", func() {
				BeforeEach(func() {
					labelRule = rules.LabelRule{
						Label: "test-label",
						IndexingFunc: func(obj map[string]any) (*string, error) {
							return nil, errors.New("test-err")
						},
					}
				})

				It("returns the error", func() {
					Expect(applyErr).To(MatchError("test-err"))
				})
			})
		})
	})

	Describe("MultiLabelRule", func() {
		var multiRule rules.MultiLabelRule

		BeforeEach(func() {
			multiRule = rules.MultiLabelRule{
				LabelRules: func(obj map[string]any) ([]rules.LabelRule, error) {
					return []rules.LabelRule{
						{
							Label: "l1",
							IndexingFunc: func(obj map[string]any) (*string, error) {
								return tools.PtrTo(obj["k1"].(string)), nil
							},
						},
						{
							Label: "l2",
							IndexingFunc: func(obj map[string]any) (*string, error) {
								return tools.PtrTo(obj["k2"].(string)), nil
							},
						},
					}, nil
				},
			}
		})

		Describe("Apply", func() {
			var (
				labels   map[string]string
				applyErr error
			)

			JustBeforeEach(func() {
				labels, applyErr = multiRule.Apply(map[string]any{
					"k1": "v1",
					"k2": "v2",
				})
			})

			It("returns a map with labels", func() {
				Expect(applyErr).NotTo(HaveOccurred())
				Expect(labels).To(Equal(map[string]string{
					"l1": "v1",
					"l2": "v2",
				}))
			})

			When("the indexing func errors", func() {
				BeforeEach(func() {
					multiRule = rules.MultiLabelRule{
						LabelRules: func(obj map[string]any) ([]rules.LabelRule, error) {
							return []rules.LabelRule{
								{
									Label: "l1",
									IndexingFunc: func(obj map[string]any) (*string, error) {
										return nil, errors.New("test-err")
									},
								},
							}, nil
						},
					}
				})

				It("returns the error", func() {
					Expect(applyErr).To(MatchError("test-err"))
				})
			})

			When("constructing nested rules fails", func() {
				BeforeEach(func() {
					multiRule = rules.MultiLabelRule{
						LabelRules: func(obj map[string]any) ([]rules.LabelRule, error) {
							return nil, errors.New("test-err")
						},
					}
				})

				It("returns the error", func() {
					Expect(applyErr).To(MatchError("test-err"))
				})
			})
		})
	})
})
