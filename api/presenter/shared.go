package presenter

import (
	"maps"
	"net/url"
	"path"
	"strconv"
	"time"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/include"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/v2/it"
)

type Lifecycle struct {
	Type string        `json:"type"`
	Data LifecycleData `json:"data"`
}

type LifecycleData struct {
	Buildpacks []string `json:"buildpacks"`
	Stack      string   `json:"stack,omitempty"`
}

type RelationshipData struct {
	GUID string `json:"guid,omitempty"`
}

func ForRelationships(relationships map[string]string) map[string]ToOneRelationship {
	return maps.Collect(it.Map2(maps.All(relationships), func(key, value string) (string, ToOneRelationship) {
		return key, ToOneRelationship{
			Data: Relationship{
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
	TotalResults int      `json:"total_results"`
	TotalPages   int      `json:"total_pages"`
	First        PageRef  `json:"first"`
	Last         PageRef  `json:"last"`
	Next         *PageRef `json:"next"`
	Previous     *PageRef `json:"previous"`
}

type PageRef struct {
	HREF string `json:"href"`
}
type Relationship struct {
	GUID string `json:"guid"`
}

type ToOneRelationship struct {
	Data Relationship `json:"data"`
}

type itemPresenter[T, S any] func(T, url.URL, ...include.Resource) S

func ForListDeprecated[T, S any](itemPresenter itemPresenter[T, S], resources []T, baseURL, requestURL url.URL, includes ...include.Resource) ListResponse[S] {
	singlePageListResult := repositories.ListResult[T]{
		PageInfo: descriptors.SinglePageInfo(len(resources), len(resources)),
		Records:  resources,
	}
	return ForList(itemPresenter, singlePageListResult, baseURL, requestURL, includes...)
}

func ForList[T, S any](itemPresenter itemPresenter[T, S], listResult repositories.ListResult[T], baseURL, requestURL url.URL, includes ...include.Resource) ListResponse[S] {
	presenters := []S{}
	for _, resource := range listResult.Records {
		presenters = append(presenters, itemPresenter(resource, baseURL))
	}

	firstQuery := requestURL.Query()
	firstQuery.Set("page", "1")
	firstQuery.Set("per_page", strconv.Itoa(listResult.PageInfo.PageSize))

	lastQuery := requestURL.Query()
	lastQuery.Set("page", strconv.Itoa(tools.Max(1, listResult.PageInfo.TotalPages)))
	lastQuery.Set("per_page", strconv.Itoa(listResult.PageInfo.PageSize))

	paginationData := PaginationData{
		TotalResults: listResult.PageInfo.TotalResults,
		TotalPages:   listResult.PageInfo.TotalPages,
		First: PageRef{
			HREF: buildURL(baseURL).appendPath(requestURL.Path).setQuery(firstQuery.Encode()).build(),
		},
		Last: PageRef{
			HREF: buildURL(baseURL).appendPath(requestURL.Path).setQuery(lastQuery.Encode()).build(),
		},
	}

	if listResult.PageInfo.PageNumber < listResult.PageInfo.TotalPages {
		nextQuery := requestURL.Query()
		nextQuery.Set("page", strconv.Itoa(listResult.PageInfo.PageNumber+1))
		nextQuery.Set("per_page", strconv.Itoa(listResult.PageInfo.PageSize))
		paginationData.Next = &PageRef{
			HREF: buildURL(baseURL).appendPath(requestURL.Path).setQuery(nextQuery.Encode()).build(),
		}
	}

	if listResult.PageInfo.PageNumber > 1 {
		prevPageNumber := tools.Min(listResult.PageInfo.PageNumber-1, listResult.PageInfo.TotalPages)
		previousQuery := requestURL.Query()
		previousQuery.Set("page", strconv.Itoa(prevPageNumber))
		previousQuery.Set("per_page", strconv.Itoa(listResult.PageInfo.PageSize))
		paginationData.Previous = &PageRef{
			HREF: buildURL(baseURL).appendPath(requestURL.Path).setQuery(previousQuery.Encode()).build(),
		}
	}

	return ListResponse[S]{
		PaginationData: paginationData,
		Resources:      presenters,
		Included:       includedResources(includes...),
	}
}

func includedResources(includes ...include.Resource) map[string][]any {
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

func formatTimestamp(t *time.Time) *string {
	if t == nil {
		return nil
	}
	return tools.PtrTo(t.UTC().Format(time.RFC3339))
}
