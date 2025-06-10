package descriptors

import (
	"errors"

	"golang.org/x/exp/constraints"
)

type PageInfo struct {
	TotalResults int
	TotalPages   int
	PageNumber   int
	PageSize     int
}

type Page[T any] struct {
	PageInfo
	Items []T
}

func SinglePageInfo(itemsCount int, pageSize int) PageInfo {
	return PageInfo{
		TotalResults: itemsCount,
		TotalPages:   1,
		PageNumber:   1,
		PageSize:     pageSize,
	}
}

func SinglePage[T any](items []T, pageSize int) Page[T] {
	return Page[T]{
		PageInfo: SinglePageInfo(len(items), pageSize),
		Items:    items,
	}
}

func GetPage[T any](items []T, pageSize int, pageNumber int) (Page[T], error) {
	var none Page[T]

	if pageSize < 1 {
		return none, errors.New("pageSize cannot be less than 1")
	}

	if pageNumber < 1 {
		return none, errors.New("pageNumber cannot be less than 1")
	}

	if pageSize >= len(items) {
		return SinglePage(items, pageSize), nil
	}

	totalResults := len(items)

	totalPages := totalResults / pageSize
	if totalResults%pageSize != 0 {
		totalPages += 1
	}
	if pageNumber > totalPages {
		return Page[T]{
			PageInfo: PageInfo{
				TotalResults: totalResults,
				TotalPages:   totalPages,
				PageNumber:   pageNumber,
				PageSize:     pageSize,
			},
		}, nil
	}

	// pageNumber is 1-based
	startIndex := pageSize * (pageNumber - 1)
	endIndex := min(totalResults, startIndex+pageSize)

	return Page[T]{
		PageInfo: PageInfo{
			TotalResults: totalResults,
			TotalPages:   totalPages,
			PageNumber:   pageNumber,
			PageSize:     pageSize,
		},
		Items: items[startIndex:endIndex],
	}, nil
}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}
