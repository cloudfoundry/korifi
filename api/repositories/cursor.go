package repositories

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
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

func ContinueOnlyList[T runtime.Object](ctx context.Context, client rest.Interface, namespace, resource string, limit int64) (string, error) {
	// Create a zero value of the target type (T)
	req := client.
		Get().
		Namespace(namespace).
		Resource(resource).
		Param("limit", stringOr(limit)).
		Param("watch", "false")

	// Stream raw response
	stream, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	respBytes, err := io.ReadAll(io.LimitReader(stream, 32*1024))
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	matches := regexp.MustCompile(`"continue":"([^"]+)"`).FindStringSubmatch(string(respBytes))
	if len(matches) != 2 {
		return "", fmt.Errorf("failed to find continue token in response: %s", string(respBytes))
	}

	return matches[1], nil
}

func stringOr(i int64) string {
	return fmt.Sprintf("%d", i)
}
