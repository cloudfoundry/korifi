package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories/include"
)

type ServiceRouteBinding struct{}

func ForServiceRouteBindingsList(baseURL, requestURL url.URL) ListResponse[any] {
	return ForList(func(a any, _ url.URL, includes ...include.Resource) any { return a }, []any{}, baseURL, requestURL)
}
