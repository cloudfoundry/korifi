package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/config"
	"code.cloudfoundry.org/korifi/version"
)

type InfoV3Response struct {
	Build       string                 `json:"build"`
	CLIVersion  InfoCLIVersion         `json:"cli_version"`
	Description string                 `json:"description"`
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Custom      map[string]interface{} `json:"custom"`

	Links map[string]Link `json:"links"`
}

type InfoCLIVersion struct {
	Minimum     string `json:"minimum"`
	Recommended string `json:"recommended"`
}

func ForInfoV3(baseURL url.URL, infoConfig config.InfoConfig) InfoV3Response {
	return InfoV3Response{
		Build:       version.Version,
		Description: infoConfig.Description,
		Name:        infoConfig.Name,
		Version:     version.Version,
		CLIVersion: InfoCLIVersion{
			Minimum:     infoConfig.MinCLIVersion,
			Recommended: infoConfig.RecommendedCLIVersion,
		},
		Custom: emptyMapIfNil(infoConfig.Custom),
		Links: map[string]Link{
			"self": {
				HRef: buildURL(baseURL).appendPath("v3/info").build(),
			},
			"support": {
				HRef: infoConfig.SupportAddress,
			},
		},
	}
}
