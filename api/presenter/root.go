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

func GetRootResponse(serverURL string) RootResponse {
	return RootResponse{
		Links: map[string]*APILink{
			"self":                {Link: Link{HREF: serverURL}},
			"bits_service":        nil,
			"cloud_controller_v2": nil,
			"cloud_controller_v3": {Link: Link{HREF: serverURL + "/v3"}, Meta: APILinkMeta{Version: "3.90.0"}},
			"network_policy_v0":   nil,
			"network_policy_v1":   nil,
			"login":               nil,
			"uaa":                 nil,
			"credhub":             nil,
			"routing":             nil,
			"logging":             nil,
			"log_cache":           nil,
			"log_stream":          nil,
			"app_ssh":             nil,
		},
		CFOnK8s: true,
	}
}
