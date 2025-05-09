package repositories

import (
	"bytes"
	"io"
	"net/http"
	"strings"
)

type discardBodyRoundTripper struct {
	delegate http.RoundTripper
}

func NewDiscardBodyRoundTripper(delegate http.RoundTripper) *discardBodyRoundTripper {
	return &discardBodyRoundTripper{
		delegate: delegate,
	}
}

func (t *discardBodyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.delegate.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if req.Method == "GET" && strings.HasPrefix(req.URL.Path, "/api/v1/namespaces/") {
		if err := resp.Body.Close(); err != nil {
			return nil, err
		}
		resp.Body = io.NopCloser(bytes.NewReader([]byte(`{}`)))
	}

	return resp, nil
}
