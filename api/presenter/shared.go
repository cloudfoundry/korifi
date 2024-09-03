package presenter

import (
	"maps"
	"net/url"
	"path"
	"time"

	"code.cloudfoundry.org/korifi/model"
	"github.com/BooleanCat/go-functional/v2/it"
)

type Lifecycle struct {
	Type string        `json:"type"`
	Data LifecycleData `json:"data"`
}

type LifecycleData struct {
	Buildpacks []string `json:"buildpacks,omitempty"`
	Stack      string   `json:"stack,omitempty"`
}

type RelationshipData struct {
	GUID string `json:"guid,omitempty"`
}

func ForRelationships(relationships map[string]string) map[string]model.ToOneRelationship {
	return maps.Collect(it.Map2(maps.All(relationships), func(key, value string) (string, model.ToOneRelationship) {
		return key, model.ToOneRelationship{
			Data: model.Relationship{
				GUID: value,
			},
		}
	}))
}

type Metadata struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

type Link struct {
	HRef   string `json:"href,omitempty"`
	Method string `json:"method,omitempty"`
}

type ListResponse[T any] struct {
	PaginationData PaginationData   `json:"pagination"`
	Resources      []T              `json:"resources"`
	Included       map[string][]any `json:"included,omitempty"`
}

type PaginationData struct {
	TotalResults int     `json:"total_results"`
	TotalPages   int     `json:"total_pages"`
	First        PageRef `json:"first"`
	Last         PageRef `json:"last"`
	Next         *int    `json:"next"`
	Previous     *int    `json:"previous"`
}

type PageRef struct {
	HREF string `json:"href"`
}

type itemPresenter[T, S any] func(T, url.URL) S

func ForList[T, S any](itemPresenter itemPresenter[T, S], resources []T, baseURL, requestURL url.URL, includes ...model.IncludedResource) ListResponse[S] {
	presenters := []S{}
	for _, resource := range resources {
		presenters = append(presenters, itemPresenter(resource, baseURL))
	}
	return ListResponse[S]{
		PaginationData: PaginationData{
			TotalResults: len(resources),
			TotalPages:   1,
			First: PageRef{
				HREF: buildURL(baseURL).appendPath(requestURL.Path).setQuery(requestURL.RawQuery).build(),
			},
			Last: PageRef{
				HREF: buildURL(baseURL).appendPath(requestURL.Path).setQuery(requestURL.RawQuery).build(),
			},
		},
		Resources: presenters,
		Included:  includedResources(includes...),
	}
}

func includedResources(includes ...model.IncludedResource) map[string][]any {
	resources := map[string][]any{}
	for _, include := range includes {
		if resources[include.Type] == nil {
			resources[include.Type] = []any{}
		}

		resources[include.Type] = append(resources[include.Type], include.Resource)
	}

	return resources
}

type buildURL url.URL

func (u buildURL) appendPath(subpath ...string) buildURL {
	rest := path.Join(subpath...)
	if u.Path == "" {
		u.Path = rest
	} else {
		u.Path = path.Join(u.Path, rest)
	}

	return u
}

func (u buildURL) setQuery(rawQuery string) buildURL {
	u.RawQuery = rawQuery

	return u
}

func (u buildURL) build() string {
	native := url.URL(u)
	nativeP := &native

	return nativeP.String()
}

func emptyMapIfNil[V any](m map[string]V) map[string]V {
	if m == nil {
		return map[string]V{}
	}
	return m
}

func emptySliceIfNil(m []string) []string {
	if m == nil {
		return []string{}
	}
	return m
}

func formatTimestamp(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
