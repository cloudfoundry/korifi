package presenter

import "net/url"

type ServiceRouteBinding struct{}

func ForServiceRouteBindingsList(baseURL, requestURL url.URL) ListResponse[any] {
	return ForList(func(a any, _ url.URL) any { return a }, []any{}, baseURL, requestURL)
}
