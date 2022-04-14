package presenter

import (
	"net/url"

	"code.cloudfoundry.org/korifi/api/repositories"
)

type BuildpackResponse struct {
	GUID      string          `json:"guid"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
	Name      string          `json:"name"`
	Filename  string          `json:"filename"`
	Stack     string          `json:"stack"`
	Position  int             `json:"position"`
	Enabled   bool            `json:"enabled"`
	Locked    bool            `json:"locked"`
	Metadata  Metadata        `json:"metadata"`
	Links     map[string]Link `json:"links"`
}

func ForBuildpack(buildpackRecord repositories.BuildpackRecord, baseURL url.URL) BuildpackResponse {
	toReturn := BuildpackResponse{
		GUID:      "",
		CreatedAt: buildpackRecord.CreatedAt,
		UpdatedAt: buildpackRecord.UpdatedAt,
		Name:      buildpackRecord.Name,
		Filename:  buildpackRecord.Name + "@" + buildpackRecord.Version,
		Stack:     buildpackRecord.Stack,
		Position:  buildpackRecord.Position,
		Enabled:   true,
		Locked:    false,
		Metadata: Metadata{
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Links: map[string]Link{},
	}

	return toReturn
}

func ForBuildpackList(buildpackRecordList []repositories.BuildpackRecord, baseURL, requestURL url.URL) ListResponse {
	buildpackResponses := make([]interface{}, 0, len(buildpackRecordList))
	for _, buildpack := range buildpackRecordList {
		buildpackResponses = append(buildpackResponses, ForBuildpack(buildpack, baseURL))
	}

	return ForList(buildpackResponses, baseURL, requestURL)
}
