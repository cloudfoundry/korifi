package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/model"
)

type ServiceRouteBinding struct{}

func ForServiceRouteBindingsList(baseURL, requestURL url.URL) ListResponse[any] {
	return ForList(func(a any, _ url.URL, includes ...model.IncludedResource) any { return a }, []any{}, baseURL, requestURL)
}
