package compare_test

import (
	"code.cloudfoundry.org/korifi/api/repositories/compare"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type sortable struct {
	sortField int
}

var _ = Describe("Sort", func() {
	var (
		s1, s2, s3 sortable
		sorter     *compare.Sorter[sortable]
		orderBy    string
		sorted     []sortable
	)

	BeforeEach(func() {
		s1 = sortable{sortField: 1}
		s2 = sortable{sortField: 2}
		s3 = sortable{sortField: 3}
		orderBy = "sortable_field"
		sorter = compare.NewSorter(func(string) func(s1, s2 sortable) int {
			return func(s1, s2 sortable) int {
				return s1.sortField - s2.sortField
			}
		})
	})

	JustBeforeEach(func() {
		sorted = sorter.Sort([]sortable{s1, s3, s2}, orderBy)
	})

	It("sorts ascending", func() {
		Expect(sorted).To(Equal([]sortable{s1, s2, s3}))
	})

	When("orderBy is descending", func() {
		BeforeEach(func() {
			orderBy = "-sortable_field"
		})

		It("sorts descending", func() {
			Expect(sorted).To(Equal([]sortable{s3, s2, s1}))
		})
	})
})
