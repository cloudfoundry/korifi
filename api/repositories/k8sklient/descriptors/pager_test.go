package descriptors_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
)

var _ = Describe("Pager", func() {
	var (
		items []string
		page  descriptors.Page[string]
	)

	BeforeEach(func() {
		items = []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	})

	Describe("SinglePage", func() {
		JustBeforeEach(func() {
			page = descriptors.SinglePage(items, 10)
		})

		It("returns a single page", func() {
			Expect(page).To(Equal(descriptors.Page[string]{
				PageInfo: descriptors.PageInfo{
					TotalResults: 8,
					TotalPages:   1,
					PageNumber:   1,
					PageSize:     10,
				},
				Items: []string{"a", "b", "c", "d", "e", "f", "g", "h"},
			}))
		})
	})

	Describe("GetPage", func() {
		var (
			err        error
			pageSize   int
			pageNumber int
		)

		BeforeEach(func() {
			pageSize = 3
			pageNumber = 2
		})

		JustBeforeEach(func() {
			page, err = descriptors.GetPage(items, pageSize, pageNumber)
		})

		It("returns the page", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(page).To(Equal(descriptors.Page[string]{
				PageInfo: descriptors.PageInfo{
					TotalResults: 8,
					TotalPages:   3,
					PageNumber:   2,
					PageSize:     3,
				},
				Items: []string{"d", "e", "f"},
			}))
		})

		When("pageSize is less than one", func() {
			BeforeEach(func() {
				pageSize = 0
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("less than 1")))
			})
		})

		When("the number of items is divisable by page size", func() {
			BeforeEach(func() {
				pageSize = 2
			})

			It("returns the page", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(page).To(Equal(descriptors.Page[string]{
					PageInfo: descriptors.PageInfo{
						TotalResults: 8,
						TotalPages:   4,
						PageNumber:   2,
						PageSize:     2,
					},
					Items: []string{"c", "d"},
				}))
			})
		})

		When("last page is smaller than page size", func() {
			BeforeEach(func() {
				pageNumber = 3
			})

			It("returns the page", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(page).To(Equal(descriptors.Page[string]{
					PageInfo: descriptors.PageInfo{
						TotalResults: 8,
						TotalPages:   3,
						PageNumber:   3,
						PageSize:     3,
					},
					Items: []string{"g", "h"},
				}))
			})
		})

		When("pageSize is equal to all the items count", func() {
			BeforeEach(func() {
				pageSize = 8
			})

			It("returns a single page", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(page).To(Equal(descriptors.Page[string]{
					PageInfo: descriptors.PageInfo{
						TotalResults: 8,
						TotalPages:   1,
						PageNumber:   1,
						PageSize:     8,
					},
					Items: []string{"a", "b", "c", "d", "e", "f", "g", "h"},
				}))
			})
		})

		When("pageSize is greater than all the items count", func() {
			BeforeEach(func() {
				pageSize = 10
			})

			It("returns a single page", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(page).To(Equal(descriptors.Page[string]{
					PageInfo: descriptors.PageInfo{
						TotalResults: 8,
						TotalPages:   1,
						PageNumber:   1,
						PageSize:     10,
					},
					Items: []string{"a", "b", "c", "d", "e", "f", "g", "h"},
				}))
			})
		})

		When("page number is less than one", func() {
			BeforeEach(func() {
				pageNumber = 0
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("less than 1")))
			})
		})

		When("the page number is greater than total pages", func() {
			BeforeEach(func() {
				pageNumber = 100
			})

			It("returns an empty page", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(page).To(Equal(descriptors.Page[string]{
					PageInfo: descriptors.PageInfo{
						TotalResults: 8,
						TotalPages:   3,
						PageNumber:   100,
						PageSize:     3,
					},
				}))
			})
		})
	})
})
