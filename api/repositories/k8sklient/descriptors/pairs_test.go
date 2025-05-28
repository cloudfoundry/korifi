package descriptors_test

import (
	"slices"

	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	"github.com/BooleanCat/go-functional/v2/it"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Pairs", func() {
	var (
		pairs descriptors.PairsList
		desc  bool
	)

	pairsGuids := func() []string {
		return slices.Collect(it.Map(slices.Values(pairs), func(pair *descriptors.Pair) string {
			return pair.Guid
		}))
	}

	BeforeEach(func() {
		desc = false
	})

	JustBeforeEach(func() {
		pairs.Sort(desc)
	})

	Describe("SortIntegerPairs", func() {
		BeforeEach(func() {
			pairs = []*descriptors.Pair{
				{Guid: "guid-2", Value: 2, ValueType: "integer"},
				{Guid: "guid-1", Value: 1, ValueType: "integer"},
				{Guid: "guid-3", Value: 3, ValueType: "integer"},
			}
		})

		It("sorts the pairs", func() {
			Expect(pairsGuids()).To(Equal([]string{"guid-1", "guid-2", "guid-3"}))
		})

		When("sorting desc", func() {
			BeforeEach(func() {
				desc = true
			})

			It("sorts the pairs in descending order", func() {
				Expect(pairsGuids()).To(Equal([]string{"guid-3", "guid-2", "guid-1"}))
			})
		})
	})

	Describe("SortPairsAsStrings", func() {
		BeforeEach(func() {
			pairs = []*descriptors.Pair{
				{Guid: "guid-2", Value: "b", ValueType: "whatever"},
				{Guid: "guid-1", Value: "a", ValueType: "whatever"},
				{Guid: "guid-3", Value: "c", ValueType: "whatever"},
			}
		})

		It("sorts the pairs", func() {
			Expect(pairsGuids()).To(Equal([]string{"guid-1", "guid-2", "guid-3"}))
		})

		When("sorting desc", func() {
			BeforeEach(func() {
				desc = true
			})

			It("sorts the pairs in descending order", func() {
				Expect(pairsGuids()).To(Equal([]string{"guid-3", "guid-2", "guid-1"}))
			})
		})
	})
})
