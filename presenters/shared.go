package presenters

import "fmt"

type Lifecycle struct {
	Data LifecycleData `json:"data"`
}

type LifecycleData struct {
	Buildpacks []string `json:"buildpacks"`
	Stack      string   `json:"stack"`
}

type Relationships map[string]Relationship

type Relationship struct {
	GUID string `json:"guid"`
}

type Metadata struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

type Link struct {
	HREF   string `json:"href,omitempty"`
	Method string `json:"method,omitempty"`
}

func prefixedLinkURL(baseURL, path string) string {
	return fmt.Sprintf("%s/%s", baseURL, path)
}
