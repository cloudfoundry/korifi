package osbapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/services/credentials"
	"code.cloudfoundry.org/korifi/model/services"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const osbapiVersion = "2.17"

type Catalog struct {
	Services []Service `json:"services"`
}

type Service struct {
	services.BrokerCatalogFeatures `json:",inline"`
	ID                             string         `json:"id"`
	Name                           string         `json:"name"`
	Description                    string         `json:"description"`
	Tags                           []string       `json:"tags"`
	Requires                       []string       `json:"requires"`
	Metadata                       map[string]any `json:"metadata"`
	DashboardClient                struct {
		Id          string `json:"id"`
		Secret      string `json:"secret"`
		RedirectUri string `json:"redirect_url"`
	} `json:"dashboard_client"`
	Plans []Plan `json:"plans"`
}

type Plan struct {
	ID               string                      `json:"id"`
	Name             string                      `json:"name"`
	Description      string                      `json:"description"`
	Metadata         map[string]any              `json:"metadata"`
	Free             bool                        `json:"free"`
	Bindable         bool                        `json:"bindable"`
	BindingRotatable bool                        `json:"binding_rotatable"`
	PlanUpdateable   bool                        `json:"plan_updateable"`
	Schemas          services.ServicePlanSchemas `json:"schemas"`
}

type Client struct {
	k8sClient client.Client
	insecure  bool
}

func NewClient(k8sClient client.Client, insecure bool) *Client {
	return &Client{k8sClient: k8sClient, insecure: insecure}
}

func (c *Client) GetCatalog(ctx context.Context, broker *korifiv1alpha1.CFServiceBroker) (*Catalog, error) {
	_, resp, err := c.newBrokerRequester().forBroker(broker).sendRequest(ctx, "/v2/catalog", http.MethodGet, nil)
	if err != nil {
		return nil, fmt.Errorf("get catalog request failed: %w", err)
	}

	catalog := &Catalog{}
	err = json.Unmarshal(resp, catalog)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal catalog: %w", err)
	}

	return catalog, nil
}

func payloadToReader(payload map[string]any) (io.Reader, error) {
	if len(payload) == 0 {
		return nil, nil
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	return bytes.NewBuffer(payloadBytes), nil
}

type brokerRequester struct {
	k8sClient client.Client
	broker    *korifiv1alpha1.CFServiceBroker
	insecure  bool
}

func (c *Client) newBrokerRequester() *brokerRequester {
	return &brokerRequester{k8sClient: c.k8sClient, insecure: c.insecure}
}

func (r *brokerRequester) forBroker(broker *korifiv1alpha1.CFServiceBroker) *brokerRequester {
	r.broker = broker
	return r
}

func (r *brokerRequester) sendRequest(ctx context.Context, requestPath string, method string, payload map[string]any) (int, []byte, error) {
	requestUrl, err := url.JoinPath(r.broker.Spec.URL, requestPath)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to build broker requestUrl for path %q: %w", requestPath, err)
	}

	payloadReader, err := payloadToReader(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("failed create payload reader: %w", err)
	}

	req, err := http.NewRequest(method, requestUrl, payloadReader)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create new HTTP request: %w", err)
	}
	req.Header.Add("X-Broker-API-Version", osbapiVersion)

	authHeader, err := r.buildAuthorizationHeaderValue(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to build Authorization request header value: %w", err)
	}
	req.Header.Add("Authorization", authHeader)

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: r.insecure}, //#nosec G402
	}}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to read body: %w", err)
	}

	if resp.StatusCode > 299 {
		return resp.StatusCode, respBody, fmt.Errorf("request returned non-OK status %d: %s", resp.StatusCode, string(respBody))
	}

	return resp.StatusCode, respBody, nil
}

func (r *brokerRequester) buildAuthorizationHeaderValue(ctx context.Context) (string, error) {
	userName, password, err := r.getCredentials(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get credentials: %w", err)
	}
	authPlain := fmt.Sprintf("%s:%s", userName, password)
	auth := base64.StdEncoding.EncodeToString([]byte(authPlain))
	return "Basic " + auth, nil
}

func (r *brokerRequester) getCredentials(ctx context.Context) (string, string, error) {
	credsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.broker.Namespace,
			Name:      r.broker.Spec.Credentials.Name,
		},
	}

	err := r.k8sClient.Get(ctx, client.ObjectKeyFromObject(credsSecret), credsSecret)
	if err != nil {
		return "", "", fmt.Errorf("failed to get credentials secret %q: %v", credsSecret.Name, err)
	}

	creds, err := credentials.GetCredentials(credsSecret)
	if err != nil {
		return "", "", fmt.Errorf("failed to get credentials from the credentials secret %q: %v", credsSecret.Name, err)
	}

	username, ok := creds[korifiv1alpha1.UsernameCredentialsKey].(string)
	if !ok {
		return "", "", fmt.Errorf("credentials secret %q does not have a string under the %q key",
			credsSecret.Name,
			korifiv1alpha1.UsernameCredentialsKey,
		)
	}

	password, ok := creds[korifiv1alpha1.PasswordCredentialsKey].(string)
	if !ok {
		return "", "", fmt.Errorf("credentials secret %q does not have a string under the %q key",
			credsSecret.Name,
			korifiv1alpha1.PasswordCredentialsKey,
		)
	}

	return username, password, nil
}
