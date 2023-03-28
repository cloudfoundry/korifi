package presenter

import (
	"net/url"
)

// ResourcePresenter should be implemented by each resource type.
// Present maps the repositories record to the presenter resource type.
type ResourcePresenter[T, S any] interface {
	Present(T) S
}

// Presenter can out the standard CF list response for any resource type.
type Presenter[T, S any] struct {
	baseURL       url.URL
	itemPresenter ResourcePresenter[T, S]
}

func New[T, S any](baseURL url.URL, itemPresenter ResourcePresenter[T, S]) Presenter[T, S] {
	return Presenter[T, S]{
		baseURL:       baseURL,
		itemPresenter: itemPresenter,
	}
}

func (p Presenter[T, S]) PresentResource(item T) S {
	return p.itemPresenter.Present(item)
}

func (p Presenter[T, S]) PresentList(items []T, requestURL url.URL) ResourcesResponse[S] {
	var presentedItems []S
	for _, item := range items {
		presentedItems = append(presentedItems, p.itemPresenter.Present(item))
	}

	return ResourcesResponse[S]{
		PaginationData: PaginationData{
			TotalResults: len(presentedItems),
			TotalPages:   1,
			First: PageRef{
				HREF: buildURL(p.baseURL).appendPath(requestURL.Path).setQuery(requestURL.RawQuery).build(),
			},
			Last: PageRef{
				HREF: buildURL(p.baseURL).appendPath(requestURL.Path).setQuery(requestURL.RawQuery).build(),
			},
		},
		Resources: presentedItems,
	}
}

type ResourcesResponse[S any] struct {
	PaginationData PaginationData `json:"pagination"`
	Resources      []S
	Included       *IncludedData `json:"included,omitempty"`
}
