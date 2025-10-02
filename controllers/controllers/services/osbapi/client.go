package osbapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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

type UnrecoverableError struct {
	Status int
}

func (c UnrecoverableError) Error() string {
	return fmt.Sprintf("The server responded with status: %d", c.Status)
}

func IgnoreGone(err error) error {
	if errors.As(err, &GoneError{}) {
		return nil
	}
	return err
}

func IsUnrecoveralbeError(err error) bool {
	if err == nil {
		return false
	}

	return errors.As(err, &UnrecoverableError{})
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
		sendRequest(ctx, "/v2/catalog", http.MethodGet, nil, nil)
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

func (c *Client) Provision(ctx context.Context, payload ProvisionPayload) (ProvisionResponse, error) {
	statusCode, respBytes, err := c.newBrokerRequester().
		forBroker(c.broker).
		async().
		sendRequest(
			ctx,
			"/v2/service_instances/"+payload.InstanceID,
			http.MethodPut,
			nil,
			payload.ProvisionRequest,
		)
	if err != nil {
		return ProvisionResponse{}, fmt.Errorf("provision request failed: %w", err)
	}
	if statusCode == http.StatusBadRequest || statusCode == http.StatusConflict || statusCode == http.StatusUnprocessableEntity {
		return ProvisionResponse{}, UnrecoverableError{Status: statusCode}
	}

	if statusCode >= 300 {
		return ProvisionResponse{}, fmt.Errorf("provision request failed with status code: %d", statusCode)
	}

	response := ProvisionResponse{
		IsAsync: statusCode == http.StatusAccepted,
	}

	err = json.Unmarshal(respBytes, &response)
	if err != nil {
		return ProvisionResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return response, nil
}

func (c *Client) Update(ctx context.Context, payload UpdatePayload) (UpdateResponse, error) {
	statusCode, respBytes, err := c.newBrokerRequester().
		forBroker(c.broker).
		async().
		sendRequest(
			ctx,
			"/v2/service_instances/"+payload.InstanceID,
			http.MethodPatch,
			nil,
			payload.UpdateRequest,
		)
	if err != nil {
		return UpdateResponse{}, fmt.Errorf("update request failed: %w", err)
	}
	if statusCode == http.StatusBadRequest || statusCode == http.StatusConflict || statusCode == http.StatusUnprocessableEntity {
		return UpdateResponse{}, UnrecoverableError{Status: statusCode}
	}

	if statusCode >= 300 {
		return UpdateResponse{}, fmt.Errorf("update request failed with status code: %d", statusCode)
	}

	response := UpdateResponse{
		IsAsync: statusCode == http.StatusAccepted,
	}

	err = json.Unmarshal(respBytes, &response)
	if err != nil {
		return UpdateResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return response, nil
}

func (c *Client) Deprovision(ctx context.Context, payload DeprovisionPayload) (ProvisionResponse, error) {
	statusCode, respBytes, err := c.newBrokerRequester().
		forBroker(c.broker).
		async().
		sendRequest(
			ctx,
			"/v2/service_instances/"+payload.ID,
			http.MethodDelete,
			map[string]string{
				"service_id": payload.ServiceId,
				"plan_id":    payload.PlanID,
			},
			nil,
		)
	if err != nil {
		return ProvisionResponse{}, fmt.Errorf("deprovision request failed: %w", err)
	}

	if statusCode == http.StatusGone {
		return ProvisionResponse{}, GoneError{}
	}

	if statusCode == http.StatusBadRequest || statusCode == http.StatusUnprocessableEntity {
		return ProvisionResponse{}, UnrecoverableError{Status: statusCode}
	}

	if statusCode >= 300 {
		return ProvisionResponse{}, fmt.Errorf("deprovision request failed with status code: %d", statusCode)
	}

	response := ProvisionResponse{
		IsAsync: statusCode == http.StatusAccepted,
	}

	err = json.Unmarshal(respBytes, &response)
	if err != nil {
		return ProvisionResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return response, nil
}

func (c *Client) GetServiceInstanceLastOperation(ctx context.Context, request GetInstanceLastOperationRequest) (LastOperationResponse, error) {
	statusCode, respBytes, err := c.newBrokerRequester().
		forBroker(c.broker).
		sendRequest(
			ctx,
			"/v2/service_instances/"+request.InstanceID+"/last_operation",
			http.MethodGet,
			map[string]string{
				"service_id": request.ServiceId,
				"plan_id":    request.PlanID,
				"operation":  request.Operation,
			},
			nil,
		)
	if err != nil {
		return LastOperationResponse{}, fmt.Errorf("getting service instance last operation request failed: %w", err)
	}

	if statusCode == http.StatusGone {
		return LastOperationResponse{}, GoneError{}
	}

	if statusCode != http.StatusOK {
		return LastOperationResponse{}, fmt.Errorf("getting service instance last operation request failed with code: %d", statusCode)
	}

	var response LastOperationResponse
	err = json.Unmarshal(respBytes, &response)
	if err != nil {
		return LastOperationResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return response, nil
}

func (c *Client) Bind(ctx context.Context, payload BindPayload) (BindResponse, error) {
	statusCode, respBytes, err := c.newBrokerRequester().
		forBroker(c.broker).
		async().
		sendRequest(
			ctx,
			"/v2/service_instances/"+payload.InstanceID+"/service_bindings/"+payload.BindingID,
			http.MethodPut,
			nil,
			payload.BindRequest,
		)
	if err != nil {
		return BindResponse{}, fmt.Errorf("bind request failed: %w", err)
	}

	if statusCode == http.StatusBadRequest || statusCode == http.StatusConflict || statusCode == http.StatusUnprocessableEntity {
		return BindResponse{}, UnrecoverableError{Status: statusCode}
	}

	if statusCode >= 300 {
		return BindResponse{}, fmt.Errorf("binding request failed with code: %d", statusCode)
	}

	response := BindResponse{
		IsAsync: statusCode == http.StatusAccepted,
	}

	err = json.Unmarshal(respBytes, &response)
	if err != nil {
		return BindResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return response, nil
}

func (c *Client) GetServiceBindingLastOperation(ctx context.Context, request GetBindingLastOperationRequest) (LastOperationResponse, error) {
	statusCode, respBytes, err := c.newBrokerRequester().
		forBroker(c.broker).
		sendRequest(
			ctx,
			"/v2/service_instances/"+request.InstanceID+"/service_bindings/"+request.BindingID+"/last_operation",
			http.MethodGet,
			map[string]string{
				"service_id": request.ServiceId,
				"plan_id":    request.PlanID,
				"operation":  request.Operation,
			},
			nil,
		)
	if err != nil {
		return LastOperationResponse{}, fmt.Errorf("getting service binding last operation request failed: %w", err)
	}

	if statusCode == http.StatusGone {
		return LastOperationResponse{}, GoneError{}
	}

	if statusCode != http.StatusOK {
		return LastOperationResponse{}, fmt.Errorf("getting service binding last operation request failed with code: %d", statusCode)
	}

	var response LastOperationResponse
	err = json.Unmarshal(respBytes, &response)
	if err != nil {
		return LastOperationResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return response, nil
}

func (c *Client) Unbind(ctx context.Context, payload UnbindPayload) (UnbindResponse, error) {
	statusCode, respBytes, err := c.newBrokerRequester().
		forBroker(c.broker).
		async().
		sendRequest(
			ctx,
			"/v2/service_instances/"+payload.InstanceID+"/service_bindings/"+payload.BindingID,
			http.MethodDelete,
			map[string]string{
				"service_id": payload.ServiceId,
				"plan_id":    payload.PlanID,
			},
			nil,
		)
	if err != nil {
		return UnbindResponse{}, fmt.Errorf("unbind request failed: %w", err)
	}

	if statusCode == http.StatusGone {
		return UnbindResponse{}, GoneError{}
	}

	if statusCode == http.StatusBadRequest || statusCode == http.StatusUnprocessableEntity {
		return UnbindResponse{}, UnrecoverableError{Status: statusCode}
	}

	if statusCode >= 300 {
		return UnbindResponse{}, fmt.Errorf("unbind request failed with status code: %d", statusCode)
	}

	response := UnbindResponse{
		IsAsync: statusCode == http.StatusAccepted,
	}
	err = json.Unmarshal(respBytes, &response)
	if err != nil {
		return UnbindResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return response, nil
}

func (c *Client) GetServiceBinding(ctx context.Context, payload BindPayload) (BindingResponse, error) {
	statusCode, respBytes, err := c.newBrokerRequester().
		forBroker(c.broker).
		sendRequest(
			ctx,
			"/v2/service_instances/"+payload.InstanceID+"/service_bindings/"+payload.BindingID,
			http.MethodGet,
			map[string]string{
				"service_id": payload.ServiceId,
				"plan_id":    payload.PlanID,
			},
			nil,
		)
	if err != nil {
		return BindingResponse{}, fmt.Errorf("fetching service binding failed: %w", err)
	}

	if statusCode == http.StatusNotFound {
		return BindingResponse{}, UnrecoverableError{Status: statusCode}
	}

	response := BindingResponse{}
	err = json.Unmarshal(respBytes, &response)
	if err != nil {
		return BindingResponse{}, fmt.Errorf("failed to unmarshal response: %w", err)
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

func (r *brokerRequester) sendRequest(ctx context.Context, requestPath string, method string, queryParams map[string]string, payload any) (int, []byte, error) {
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

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-Broker-API-Version", osbapiVersion)

	queryValues := req.URL.Query()
	for queryParam, queryParamValue := range queryParams {
		if queryParamValue == "" {
			continue
		}
		queryValues.Add(queryParam, queryParamValue)
	}
	if r.acceptsIncomplete {
		queryValues.Add("accepts_incomplete", "true")
	}
	req.URL.RawQuery = queryValues.Encode()

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
