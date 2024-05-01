package presenter

import "net/url"

type InfoV3Response struct {
	Build       string                 `json:"build"`
	CLIVersion  InfoCLIVersion         `json:"cli_version"`
	Description string                 `json:"description"`
	Name        string                 `json:"name"`
	Version     int                    `json:"version"`
	Custom      map[string]interface{} `json:"custom"`

	Links map[string]Link `json:"links"`
}

type InfoCLIVersion struct {
	Minimum     string `json:"minimum"`
	Recommended string `json:"recommended"`
}

func ForInfoV3(baseURL url.URL) InfoV3Response {
	return InfoV3Response{
		Custom: make(map[string]interface{}),
		Links: map[string]Link{
			"self": {
				HRef: buildURL(baseURL).appendPath("v3/info").build(),
			},
			"support": {
				HRef: "https://www.cloudfoundry.org/technology/korifi/",
			},
		},
	}
}
