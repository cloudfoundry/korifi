package descriptors_test

import (
	"slices"

	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("TableResultSetDescriptor", func() {
	var (
		results  descriptors.TableResultSetDescriptor
		guids    []string
		column   string
		guidsErr error
	)

	BeforeEach(func() {
		results = descriptors.TableResultSetDescriptor{
			Table: &metav1.Table{
				ColumnDefinitions: []metav1.TableColumnDefinition{
					{Name: "Name", Type: "string"},
					{Name: "Age", Type: "integer"},
					{Name: "Hobbies", Type: "array"},
					{Name: "WorkAddress", Type: "string"},
					{Name: "WorkExperience", Type: "integer"},
				},
				Rows: []metav1.TableRow{
					{Cells: []any{"Alice", 30, []string{"reading", "hiking"}, "Alice LTD", 2}},
					{Cells: []any{"Bob", 25, []string{"gaming", "cooking"}, "Bob&Jim Inc", 1}},
					{Cells: []any{"Charlie", 35, []string{"traveling", "photography"}, "Charlie Studios", 3}},
					{Cells: []any{"Dave", 35, []string{"music", "sports"}, nil, nil}},
				},
			},
		}
	})

	Describe("GUIDs", func() {
		JustBeforeEach(func() {
			guids, guidsErr = results.GUIDs()
		})

		It("returns the values from the Name column as guids", func() {
			Expect(guidsErr).NotTo(HaveOccurred())
			Expect(guids).To(Equal([]string{"Alice", "Bob", "Charlie", "Dave"}))
		})
	})

	Describe("Sort", func() {
		var (
			desc    bool
			sortErr error
		)

		BeforeEach(func() {
			column = ""
			desc = false
		})

		JustBeforeEach(func() {
			sortErr = results.Sort(column, desc)
			guids, guidsErr = results.GUIDs()
			Expect(guidsErr).NotTo(HaveOccurred())
		})

		Describe("by Age", func() {
			BeforeEach(func() {
				column = "Age"
			})

			It("returns the guids sorted by the specified column", func() {
				Expect(sortErr).NotTo(HaveOccurred())
				Expect(guids).To(Equal([]string{"Bob", "Alice", "Charlie", "Dave"}))
			})

			When("sorting in descending order", func() {
				BeforeEach(func() {
					desc = true
				})

				It("returns the guids sorted in descending order", func() {
					Expect(sortErr).NotTo(HaveOccurred())
					Expect(guids).To(Equal([]string{"Charlie", "Dave", "Alice", "Bob"}))
				})
			})
		})

		Describe("by Name", func() {
			BeforeEach(func() {
				column = "Name"
			})

			It("returns the guids sorted by the specified column", func() {
				Expect(sortErr).NotTo(HaveOccurred())
				Expect(guids).To(Equal([]string{"Alice", "Bob", "Charlie", "Dave"}))
			})

			When("sorting in descending order", func() {
				BeforeEach(func() {
					desc = true
				})

				It("returns the guids sorted in descending order", func() {
					Expect(sortErr).NotTo(HaveOccurred())
					Expect(guids).To(Equal([]string{"Dave", "Charlie", "Bob", "Alice"}))
				})
			})
		})

		Describe("Sort by WorkAddress", func() {
			BeforeEach(func() {
				column = "WorkAddress"
			})

			It("returns the guids sorted by the specified column", func() {
				Expect(sortErr).NotTo(HaveOccurred())
				Expect(guids).To(Equal([]string{"Dave", "Alice", "Bob", "Charlie"}))
			})

			When("sorting in descending order", func() {
				BeforeEach(func() {
					desc = true
				})

				It("returns the guids sorted in descending order", func() {
					Expect(sortErr).NotTo(HaveOccurred())
					Expect(guids).To(Equal([]string{"Charlie", "Bob", "Alice", "Dave"}))
				})
			})
		})

		Describe("Sort by WorkExperience", func() {
			BeforeEach(func() {
				column = "WorkExperience"
			})

			It("returns the guids sorted by the specified column", func() {
				Expect(sortErr).NotTo(HaveOccurred())
				Expect(guids).To(Equal([]string{"Dave", "Bob", "Alice", "Charlie"}))
			})

			When("sorting in descending order", func() {
				BeforeEach(func() {
					desc = true
				})

				It("returns the guids sorted in descending order", func() {
					Expect(sortErr).NotTo(HaveOccurred())
					Expect(guids).To(Equal([]string{"Charlie", "Alice", "Bob", "Dave"}))
				})
			})
		})

		Describe("by Hobbies", func() {
			BeforeEach(func() {
				column = "Hobbies"
			})

			It("errors with unsupported column type", func() {
				Expect(sortErr).To(MatchError(ContainSubstring(`unsupported column type "array" for sorting`)))
			})
		})

		When("sort column is not found", func() {
			BeforeEach(func() {
				column = "NonExistentColumn"
			})

			It("returns an error", func() {
				Expect(sortErr).To(MatchError(ContainSubstring("not found")))
			})
		})
	})

	Describe("Filter", func() {
		var filterErr error
		BeforeEach(func() {
			column = "Hobbies"
		})

		JustBeforeEach(func() {
			filterErr = results.Filter(column, func(value any) bool {
				hobbies := value.([]string)
				return slices.Contains(hobbies, "hiking") || slices.Contains(hobbies, "gaming")
			})
			guids, guidsErr = results.GUIDs()
			Expect(guidsErr).NotTo(HaveOccurred())
		})

		It("filters the table by specified column", func() {
			Expect(filterErr).NotTo(HaveOccurred())
			Expect(guids).To(Equal([]string{"Alice", "Bob"}))
		})

		When("filter column is not found", func() {
			BeforeEach(func() {
				column = "NonExistentColumn"
			})

			It("returns an error", func() {
				Expect(filterErr).To(MatchError(ContainSubstring("not found")))
			})
		})
	})
})
