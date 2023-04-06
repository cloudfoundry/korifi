package presenter

import "net/url"

type APILink struct {
	Link
	Meta APILinkMeta `json:"meta"`
}

type APILinkMeta struct {
	Version string `json:"version"`
}

type RootResponse struct {
	Links   map[string]*APILink `json:"links"`
	CFOnK8s bool                `json:"cf_on_k8s"`
}

const V3APIVersion = "3.117.0+cf-k8s"

func ForRoot(baseURL url.URL) RootResponse {
	return RootResponse{
		Links: map[string]*APILink{
			"self": {
				Link: Link{
					HRef: buildURL(baseURL).build(),
				},
			},
			"bits_service":        nil,
			"cloud_controller_v2": nil,
			"cloud_controller_v3": {
				Link: Link{
					HRef: buildURL(baseURL).appendPath("v3").build(),
				},
				Meta: APILinkMeta{
					Version: V3APIVersion,
				},
			},
			"network_policy_v0": nil,
			"network_policy_v1": nil,
			"login": {
				Link: Link{
					HRef: buildURL(baseURL).build(),
				},
			},
			"uaa":     nil,
			"credhub": nil,
			"routing": nil,
			"logging": nil,
			"log_cache": {
				Link: Link{
					HRef: buildURL(baseURL).build(),
				},
			},
			"log_stream": nil,
			"app_ssh":    nil,
		},
		CFOnK8s: true,
	}
}

type RootV3Response struct {
	Links map[string]Link `json:"links"`
}

func ForRootV3(baseURL url.URL) RootV3Response {
	return RootV3Response{
		Links: map[string]Link{
			"self": {
				HRef: buildURL(baseURL).appendPath("v3").build(),
			},
		},
	}
}
