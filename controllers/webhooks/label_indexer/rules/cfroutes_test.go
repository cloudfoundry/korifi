package rules_test

import (
	"code.cloudfoundry.org/korifi/controllers/webhooks/label_indexer/rules"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("DestinationAppGUIDLabelRules", func() {
	var (
		obj    map[string]any
		err    error
		result []rules.LabelRule
	)
	BeforeEach(func() {
		obj = map[string]any{
			"spec": map[string]any{
				"destinations": []any{
					map[string]any{"appRef": map[string]any{"name": "app-guid-1"}},
					map[string]any{"appRef": map[string]any{"name": "app-guid-2"}},
				},
			},
		}
	})

	JustBeforeEach(func() {
		result, err = rules.DestinationAppGuidLabelRules(obj)
	})

	It("returns a list of rules with the correct labels", func() {
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ConsistOf(
			MatchFields(IgnoreExtras, Fields{
				"Label": Equal("korifi.cloudfoundry.org/destination-app-guid-app-guid-1"),
			}),
			MatchFields(IgnoreExtras, Fields{
				"Label": Equal("korifi.cloudfoundry.org/destination-app-guid-app-guid-2"),
			}),
		))
	})

	When("the object has no destinations", func() {
		BeforeEach(func() {
			obj = map[string]any{
				"spec": map[string]any{
					"destinations": []any{},
				},
			}
		})

		It("returns an empty list of rules", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})

	When("the object has destinations with empty app GUIDs", func() {
		BeforeEach(func() {
			obj = map[string]any{
				"spec": map[string]any{
					"destinations": []any{
						map[string]any{"appRef": map[string]any{"name": ""}},
					},
				},
			}
		})

		It("returns an empty list of rules", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})

	When("the object is empty", func() {
		BeforeEach(func() {
			obj = map[string]any{}
		})

		It("returns an empty list of rules", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})
})
