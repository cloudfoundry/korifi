package osbapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const osbapiVersion = "2.17"

type GoneError struct{}

func (g GoneError) Error() string {
	return "The operation resource is gone"
}

type Client struct {
	broker     Broker
	httpClient *http.Client
}

func NewClient(broker Broker, httpClient *http.Client) *Client {
	return &Client{
		broker:     broker,
		httpClient: httpClient,
	}
}

func (c *Client) GetCatalog(ctx context.Context) (Catalog, error) {
	statusCode, resp, err := c.newBrokerRequester().
		forBroker(c.broker).
		sendRequest(ctx, "/v2/catalog", http.MethodGet, nil)
	if err != nil {
		return Catalog{}, fmt.Errorf("get catalog request failed: %w", err)
	}

	if statusCode > 300 {
		return Catalog{}, fmt.Errorf("getting service catalog request failed with status code: %d", statusCode)
	}

	var catalog Catalog
	err = json.Unmarshal(resp, &catalog)
	if err != nil {
		return Catalog{}, fmt.Errorf("failed to unmarshal catalog: %w", err)
	}

	return catalog, nil
}

func (c *Client) Provision(ctx context.Context, payload InstanceProvisionPayload) (ServiceInstanceOperationResponse, error) {
	statusCode, respBytes, err := c.newBrokerRequester().
		forBroker(c.broker).
		async().
		sendRequest(
			ctx,
			"/v2/service_instances/"+payload.InstanceID,
			http.MethodPut,
			payload.InstanceProvisionRequest,
		)
	if err != nil {
		return ServiceInstanceOperationResponse{}, fmt.Errorf("provision request failed: %w", err)
	}

	if statusCode >= 300 {
		return ServiceInstanceOperationResponse{}, fmt.Errorf("provision request failed with status code: %d", statusCode)
	}

	response := ServiceInstanceOperationResponse{}
	if statusCode == http.StatusCreated {
		response.Complete = true
	}

	err = json.Unmarshal(respBytes, &response)
	if err != nil {
		return ServiceInstanceOperationResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return response, nil
}

func (c *Client) Deprovision(ctx context.Context, payload InstanceDeprovisionPayload) (ServiceInstanceOperationResponse, error) {
	statusCode, respBytes, err := c.newBrokerRequester().
		forBroker(c.broker).
		async().
		sendRequest(
			ctx,
			"/v2/service_instances/"+payload.ID,
			http.MethodDelete,
			payload.InstanceDeprovisionRequest,
		)
	if err != nil {
		return ServiceInstanceOperationResponse{}, fmt.Errorf("deprovision request failed: %w", err)
	}

	if statusCode >= 300 {
		return ServiceInstanceOperationResponse{}, fmt.Errorf("deprovision request failed with status code: %d", statusCode)
	}

	var response ServiceInstanceOperationResponse
	err = json.Unmarshal(respBytes, &response)
	if err != nil {
		return ServiceInstanceOperationResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return response, nil
}

func (c *Client) GetServiceInstanceLastOperation(ctx context.Context, payload GetLastOperationPayload) (LastOperationResponse, error) {
	statusCode, respBytes, err := c.newBrokerRequester().
		forBroker(c.broker).
		sendRequest(
			ctx,
			"/v2/service_instances/"+payload.ID+"/last_operation",
			http.MethodGet,
			payload.GetLastOperationRequest,
		)
	if err != nil {
		return LastOperationResponse{}, fmt.Errorf("getting last operation request failed: %w", err)
	}

	if statusCode == http.StatusGone {
		return LastOperationResponse{}, GoneError{}
	}

	if statusCode != http.StatusOK {
		return LastOperationResponse{}, fmt.Errorf("getting last operation request failed with code: %d", statusCode)
	}

	var response LastOperationResponse
	err = json.Unmarshal(respBytes, &response)
	if err != nil {
		return LastOperationResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return response, nil
}

func payloadToReader(payload any) (io.Reader, error) {
	if payload == nil {
		return nil, nil
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	return bytes.NewBuffer(payloadBytes), nil
}

type brokerRequester struct {
	broker            Broker
	acceptsIncomplete bool
	httpClient        *http.Client
}

func (c *Client) newBrokerRequester() *brokerRequester {
	return &brokerRequester{httpClient: c.httpClient}
}

func (r *brokerRequester) forBroker(broker Broker) *brokerRequester {
	r.broker = broker
	return r
}

func (r *brokerRequester) async() *brokerRequester {
	r.acceptsIncomplete = true
	return r
}

func (r *brokerRequester) sendRequest(ctx context.Context, requestPath string, method string, payload any) (int, []byte, error) {
	requestUrl, err := url.JoinPath(r.broker.URL, requestPath)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to build broker requestUrl for path %q: %w", requestPath, err)
	}

	parsedURL, err := url.Parse(requestUrl)
	if err != nil {
		panic(err)
	}

	payloadReader, err := payloadToReader(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("failed create payload reader: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, parsedURL.String(), payloadReader)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create new HTTP request: %w", err)
	}
	req.Header.Add("X-Broker-API-Version", osbapiVersion)
	if r.acceptsIncomplete {
		queryValues := req.URL.Query()
		queryValues.Add("accepts_incomplete", "true")
		req.URL.RawQuery = queryValues.Encode()
	}

	authHeader, err := r.buildAuthorizationHeaderValue()
	if err != nil {
		return 0, nil, fmt.Errorf("failed to build Authorization request header value: %w", err)
	}
	req.Header.Add("Authorization", authHeader)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to read body: %w", err)
	}

	return resp.StatusCode, respBody, nil
}

func (r *brokerRequester) buildAuthorizationHeaderValue() (string, error) {
	authPlain := fmt.Sprintf("%s:%s", r.broker.Username, r.broker.Password)
	auth := base64.StdEncoding.EncodeToString([]byte(authPlain))
	return "Basic " + auth, nil
}
