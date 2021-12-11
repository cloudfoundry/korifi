package presenter

import "net/url"

type ServiceRouteBinding struct{}

func ForServiceRouteBindingsList(baseURL, requestURL url.URL) ListResponse {
	return ForList([]interface{}{}, baseURL, requestURL)
}
