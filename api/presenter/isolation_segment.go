package presenter

import (
	"net/url"
)

type IsolationSegmentResponse struct{}

func ForIsolationSegment(apiBaseURL url.URL) IsolationSegmentResponse {
	return IsolationSegmentResponse{}
}
