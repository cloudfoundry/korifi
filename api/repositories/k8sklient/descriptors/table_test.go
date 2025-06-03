package descriptors_test

import (
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("TableResultSetDescriptor", func() {
	var (
		results  descriptors.TableResultSetDescriptor
		guids    []string
		guidsErr error
	)

	BeforeEach(func() {
		results = descriptors.TableResultSetDescriptor{
			Table: &metav1.Table{
				ColumnDefinitions: []metav1.TableColumnDefinition{
					{Name: "Name", Type: "string"},
					{Name: "Age", Type: "integer"},
					{Name: "Hobbies", Type: "array"},
				},
				Rows: []metav1.TableRow{
					{Cells: []any{"Alice", 30, []string{"reading", "hiking"}}},
					{Cells: []any{"Bob", 25, []string{"gaming", "cooking"}}},
					{Cells: []any{"Charlie", 35, []string{"traveling", "photography"}}},
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
			Expect(guids).To(Equal([]string{"Alice", "Bob", "Charlie"}))
		})
	})

	Describe("Sort", func() {
		var (
			column  string
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
				Expect(guids).To(Equal([]string{"Bob", "Alice", "Charlie"}))
			})

			Context("when sorting in descending order", func() {
				BeforeEach(func() {
					desc = true
				})

				It("returns the guids sorted in descending order", func() {
					Expect(sortErr).NotTo(HaveOccurred())
					Expect(guids).To(Equal([]string{"Charlie", "Alice", "Bob"}))
				})
			})

			Context("when the column does not exist", func() {
				BeforeEach(func() {
					column = "NonExistentColumn"
				})

				It("returns an error", func() {
					Expect(sortErr).To(MatchError("column NonExistentColumn not found"))
				})
			})
		})

		Describe("by Name", func() {
			BeforeEach(func() {
				column = "Name"
			})

			It("returns the guids sorted by the specified column", func() {
				Expect(sortErr).NotTo(HaveOccurred())
				Expect(guids).To(Equal([]string{"Alice", "Bob", "Charlie"}))
			})

			Context("when sorting in descending order", func() {
				BeforeEach(func() {
					desc = true
				})

				It("returns the guids sorted in descending order", func() {
					Expect(sortErr).NotTo(HaveOccurred())
					Expect(guids).To(Equal([]string{"Charlie", "Bob", "Alice"}))
				})
			})
		})

		Describe("by Hobbies", func() {
			BeforeEach(func() {
				column = "Hobbies"
			})

			It("returns the guids sorted by the specified column", func() {
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
})
