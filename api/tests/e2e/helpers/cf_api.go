package helpers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	. "github.com/onsi/gomega"
)

type QueryParams map[string]string

type APIRequest struct {
	ServerURL   string
	Method      string
	Endpoint    string
	AuthHeader  string
	Body        interface{}
	QueryParams QueryParams
	Response    *http.Response
	Error       error
}

func NewCFAPI(serverURL string) APIRequest {
	return APIRequest{
		ServerURL: serverURL,
	}
}

func (r APIRequest) Request(method, endpoint string) APIRequest {
	return APIRequest{
		ServerURL: r.ServerURL,
		Method:    method,
		Endpoint:  endpoint,
	}
}

func (r APIRequest) WithBody(body interface{}) APIRequest {
	r.Body = body
	return r
}

func (r APIRequest) WithQueryParams(params QueryParams) APIRequest {
	r.QueryParams = params
	return r
}

func (r APIRequest) DoWithAuth(authHeader string) APIRequest {
	r.AuthHeader = authHeader
	return r.do()
}

func (r APIRequest) Do() APIRequest {
	return r.do()
}

func (r APIRequest) do() APIRequest {
	var bodyReader io.Reader
	if r.Body != nil {
		bodyBytes, err := json.Marshal(r.Body)
		if err != nil {
			r.Error = err
			return r
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	query := url.Values{}
	for key, val := range r.QueryParams {
		query.Set(key, val)
	}

	req, err := http.NewRequest(r.Method, r.ServerURL+r.Endpoint+"?"+query.Encode(), bodyReader)
	if err != nil {
		r.Error = err
		return r
	}

	req.Header.Add("Authorization", r.AuthHeader)

	r.Response, r.Error = http.DefaultClient.Do(req)

	return r
}

func (r APIRequest) ValidateStatus(expectedStatus int) APIRequest {
	ExpectWithOffset(2, r.Raw().StatusCode).To(Equal(expectedStatus))
	return r
}

func (r APIRequest) Raw() *http.Response {
	ExpectWithOffset(2, r.Error).NotTo(HaveOccurred())
	return r.Response
}

func (r APIRequest) Status() int {
	return r.Raw().StatusCode
}

func (r APIRequest) DecodeResponseBody(receiver interface{}) {
	defer r.Response.Body.Close()
	ExpectWithOffset(2, json.NewDecoder(r.Response.Body).Decode(receiver)).To(Succeed())
}
