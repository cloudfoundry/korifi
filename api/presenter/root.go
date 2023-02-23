package presenter

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

func GetRootResponse(serverURL string) RootResponse {
	return RootResponse{
		Links: map[string]*APILink{
			"self":                {Link: Link{HRef: serverURL}},
			"bits_service":        nil,
			"cloud_controller_v2": nil,
			"cloud_controller_v3": {Link: Link{HRef: serverURL + "/v3"}, Meta: APILinkMeta{Version: V3APIVersion}},
			"network_policy_v0":   nil,
			"network_policy_v1":   nil,
			"login":               {Link: Link{HRef: "https://uaa-127-0-0-1.nip.io"}},
			"uaa":                 {Link: Link{HRef: "https://uaa-127-0-0-1.nip.io"}},
			"credhub":             nil,
			"routing":             nil,
			"logging":             nil,
			"log_cache":           {Link: Link{HRef: serverURL}},
			"log_stream":          nil,
			"app_ssh":             nil,
		},
	}
}
