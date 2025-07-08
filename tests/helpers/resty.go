package helpers

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-resty/resty/v2"
)

type actualRestyResponse struct {
	*resty.Response
}

func NewActualRestyResponse(response *resty.Response) actualRestyResponse {
	return actualRestyResponse{Response: response}
}

func (m actualRestyResponse) GomegaString() string {
	return fmt.Sprintf(`
    Request: %s %s
    Request headers:
        %s
    Request body:
        %+v
    HTTP Status code: %d
    Response headers:
        %s
    Response body:
        %s`,
		m.Request.Method, m.Request.URL,
		headersString(m.Request.Header),
		objectToPrettyJson(m.Request.Body),
		m.StatusCode(),
		headersString(m.Header()),
		formatAsPrettyJson(m.Body()),
	)
}

func headersString(headers http.Header) string {
	var s []string
	for k := range headers {
		s = append(s, formatHeader(k, headers.Get(k)))
	}
	return strings.Join(s, "\n        ")
}

func formatHeader(key, value string) string {
	if key == "Authorization" {
		return fmt.Sprintf("%s: %s", key, formatAuthorizationValue(value))
	}

	return fmt.Sprintf("%s: %s", key, value)
}

func formatAuthorizationValue(value string) string {
	substrings := strings.Split(value, " ")
	if len(substrings) != 2 {
		return value
	}

	return fmt.Sprintf("%s %s", substrings[0], "<Redacted>")
}

func objectToPrettyJson(obj any) string {
	prettyJson, err := json.MarshalIndent(obj, "        ", "  ")
	if err != nil {
		return fmt.Sprintf("%+v", obj)
	}

	return string(prettyJson)
}

func formatAsPrettyJson(b []byte) string {
	var prettyBuf bytes.Buffer
	if err := json.Indent(&prettyBuf, b, "        ", "  "); err != nil {
		return string(b)
	}

	return prettyBuf.String()
}

type CorrelatedRestyClient struct {
	*resty.Client

	getCorrelationId func() string
}

func NewCorrelatedRestyClient(apiServerRoot string, getCorrelationId func() string) *CorrelatedRestyClient {
	return &CorrelatedRestyClient{
		Client:           resty.New().SetBaseURL(apiServerRoot),
		getCorrelationId: getCorrelationId,
	}
}

func (c *CorrelatedRestyClient) R() Request {
	request := c.Client.R()
	request.SetHeader("X-Correlation-ID", c.getCorrelationId())

	return &PaginationAwareRequest{
		client:  c.Client,
		request: request,
	}
}

// Request is an interface extracted from resty.Response
type Request interface {
	SetResult(res any) Request
	SetBody(body any) Request
	SetError(err any) Request
	SetHeader(header, value string) Request
	SetPathParam(param, value string) Request
	SetFiles(files map[string]string) Request
	SetFile(param, filePath string) Request
	Get(url string) (*resty.Response, error)
	Post(url string) (*resty.Response, error)
	Put(url string) (*resty.Response, error)
	Delete(url string) (*resty.Response, error)
	Patch(url string) (*resty.Response, error)
	SetQueryParam(param, value string) Request
	SetQueryParams(params map[string]string) Request
}

var _ Request = &PaginationAwareRequest{}

type PaginationAwareRequest struct {
	client  *resty.Client
	request *resty.Request
}

func (r *PaginationAwareRequest) SetResult(res any) Request {
	r.request.SetResult(res)
	return r
}

func (r *PaginationAwareRequest) SetBody(body any) Request {
	r.request.SetBody(body)
	return r
}

func (r *PaginationAwareRequest) SetError(err any) Request {
	r.request.SetError(err)
	return r
}

func (r *PaginationAwareRequest) SetHeader(header, value string) Request {
	r.request.SetHeader(header, value)
	return r
}

func (r *PaginationAwareRequest) SetPathParam(param, value string) Request {
	r.request.SetPathParam(param, value)
	return r
}

func (r *PaginationAwareRequest) SetFiles(files map[string]string) Request {
	r.request.SetFiles(files)
	return r
}

func (r *PaginationAwareRequest) SetFile(param, filePath string) Request {
	r.request.SetFile(param, filePath)
	return r
}

func (r *PaginationAwareRequest) Post(url string) (*resty.Response, error) {
	return r.request.Post(url)
}

func (r *PaginationAwareRequest) Put(url string) (*resty.Response, error) {
	return r.request.Put(url)
}

func (r *PaginationAwareRequest) Delete(url string) (*resty.Response, error) {
	return r.request.Delete(url)
}

func (r *PaginationAwareRequest) Patch(url string) (*resty.Response, error) {
	return r.request.Patch(url)
}

func (r *PaginationAwareRequest) SetQueryParam(param, value string) Request {
	r.request.SetQueryParam(param, value)
	return r
}

func (r *PaginationAwareRequest) SetQueryParams(params map[string]string) Request {
	r.request.SetQueryParams(params)
	return r
}

func (r *PaginationAwareRequest) Get(url string) (*resty.Response, error) {
	if !pointsToStructWithResourcesField(r.request.Result) {
		return r.request.Get(url)
	}

	allResourcesResp, err := r.collectAllResources(url)
	if err != nil {
		return nil, fmt.Errorf("error collecting all resources: %w", err)
	}

	response := map[string]any{
		"resources": allResourcesResp.Resources,
	}

	respBytes, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("error marshalling resources to JSON: %w", err)
	}

	err = json.Unmarshal(respBytes, r.request.Result)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling resources JSON to result: %w", err)
	}

	return &resty.Response{
		Request: r.request,
		RawResponse: &http.Response{
			Status:     "OK",
			StatusCode: http.StatusOK,
			Header:     allResourcesResp.Header,
		},
	}, nil
}

func pointsToStructWithResourcesField(v any) bool {
	var none any

	if v == none {
		return false
	}

	value := reflect.ValueOf(v)
	if value.Kind() != reflect.Ptr {
		return false
	}

	derefedValue := value.Elem()
	if derefedValue.Kind() != reflect.Struct {
		return false
	}

	return derefedValue.FieldByName("Resources").IsValid()
}

func (r *PaginationAwareRequest) collectAllResources(url string) (ResourcesResponse, error) {
	thisPageResult := &resourceList{}
	resp, err := r.cloneRequest().SetResult(thisPageResult).Get(url)
	if err != nil {
		return ResourcesResponse{}, err
	}

	if resp.StatusCode() != http.StatusOK {
		return ResourcesResponse{}, fmt.Errorf("unexpected status code %d for %s", resp.StatusCode(), url)
	}

	respHeader := resp.Header()
	allResources := thisPageResult.Resources
	if thisPageResult.Pagination.Next != nil {
		nextPageResults, err := r.collectAllResources(thisPageResult.Pagination.Next.Href)
		if err != nil {
			return ResourcesResponse{}, fmt.Errorf("error collecting resources from next page: %w", err)
		}
		respHeader = nextPageResults.Header
		allResources = append(allResources, nextPageResults.Resources...)
	}

	return ResourcesResponse{
		Resources: allResources,
		Header:    respHeader,
	}, nil
}

type ResourcesResponse struct {
	Resources []any
	Header    http.Header
}

func (r *PaginationAwareRequest) cloneRequest() *resty.Request {
	return r.client.R().
		SetPathParams(r.request.PathParams)
}

func (c *CorrelatedRestyClient) SetAuthToken(token string) *CorrelatedRestyClient {
	c.Client.SetAuthToken(token)
	return c
}

func (c *CorrelatedRestyClient) SetTLSClientConfig(config *tls.Config) *CorrelatedRestyClient {
	c.Client.SetTLSClientConfig(config)
	return c
}

func (c *CorrelatedRestyClient) SetAuthScheme(scheme string) *CorrelatedRestyClient {
	c.Client.SetAuthScheme(scheme)
	return c
}

type resourceList struct {
	Pagination Pagination `json:"pagination"`
	Resources  []any      `json:"resources"`
}

type Pagination struct {
	Next *PaginationLink `json:"next,omitempty"`
}

type PaginationLink struct {
	Href string `json:"href"`
}
