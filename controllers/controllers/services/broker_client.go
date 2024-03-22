package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BrokerClient interface {
	ProvisionServiceInstance(context.Context, *korifiv1alpha1.CFServiceInstance) error
	GetServiceInstanceLastOperation(context.Context, *korifiv1alpha1.CFServiceInstance) (LastOperation, error)
	DeprovisionServiceInstance(context.Context, *korifiv1alpha1.CFServiceInstance) error

	BindService(context.Context, *korifiv1alpha1.CFServiceBinding) error
	GetServiceBinding(context.Context, *korifiv1alpha1.CFServiceBinding) (ServiceBinding, error)
	GetServiceBindingLastOperation(context.Context, *korifiv1alpha1.CFServiceBinding) (LastOperation, error)
	UnbindService(context.Context, *korifiv1alpha1.CFServiceBinding) error
}

type ServiceBinding struct {
	Credentials map[string]any `json:"credentials"`
}

type LastOperation struct {
	Exists      bool
	State       string
	Description string
}

type brokerClient struct {
	k8sClient     client.Client
	rootNamespace string
}

func NewBrokerClient(k8sClient client.Client, rootNamespace string) BrokerClient {
	return &brokerClient{
		k8sClient:     k8sClient,
		rootNamespace: rootNamespace,
	}
}

func (c *brokerClient) ProvisionServiceInstance(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance) error {
	plan, err := c.getServicePlan(ctx, cfServiceInstance)
	if err != nil {
		return err
	}

	offering, err := c.getServiceOffering(ctx, plan)
	if err != nil {
		return err
	}

	broker, err := c.getBroker(ctx, plan)
	if err != nil {
		return err
	}

	provisionRequest := map[string]any{
		"service_id": offering.Spec.Broker_catalog.Id,
		"plan_id":    plan.Spec.Broker_catalog.Id,
	}

	if cfServiceInstance.Spec.Parameters != nil {
		paramsMap := map[string]any{}
		err = json.Unmarshal(cfServiceInstance.Spec.Parameters.Raw, &paramsMap)
		if err != nil {
			return fmt.Errorf("failed to unmarshal service instance parameters: %w", err)
		}
		provisionRequest["parameters"] = paramsMap
	}

	provisionUrl, err := url.JoinPath(broker.Spec.URL, "v2", "service_instances", cfServiceInstance.Name)
	if err != nil {
		return fmt.Errorf("failed to construct service broker provision url: %w", err)
	}
	provisionUrl += "?" + url.Values{"accepts_incomplete": {"true"}}.Encode()

	respCode, resp, err := sendRequest(provisionUrl, http.MethodPut, provisionRequest)
	if err != nil {
		return fmt.Errorf("provision request failed: %w", err)
	}

	if respCode > 299 {
		return fmt.Errorf("provision request returned non-OK status %d: %s", respCode, string(resp))
	}

	return nil
}

func (c *brokerClient) GetServiceInstanceLastOperation(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (LastOperation, error) {
	plan, err := c.getServicePlan(ctx, cfServiceInstance)
	if err != nil {
		return LastOperation{}, err
	}

	broker, err := c.getBroker(ctx, plan)
	if err != nil {
		return LastOperation{}, err
	}

	stateUrl, err := url.JoinPath(broker.Spec.URL, "v2", "service_instances", cfServiceInstance.Name, "last_operation")
	if err != nil {
		return LastOperation{}, fmt.Errorf("failed to construct service broker last operation url: %w", err)
	}

	respCode, resp, err := sendRequest(stateUrl, http.MethodGet, nil)
	if err != nil {
		return LastOperation{}, fmt.Errorf("last operation request failed: %w", err)
	}

	if respCode == http.StatusNotFound {
		return LastOperation{
			Exists: false,
		}, nil
	}

	respMap := map[string]string{}
	err = json.Unmarshal(resp, &respMap)
	if err != nil {
		return LastOperation{}, fmt.Errorf("failed to unmarshal last operation response: %w", err)
	}

	if respCode > 299 {
		return LastOperation{}, fmt.Errorf("last operation request failed: status code: %d: %v", respCode, respMap)
	}

	return LastOperation{
		Exists:      true,
		State:       respMap["state"],
		Description: respMap["description"],
	}, nil
}

func (c *brokerClient) DeprovisionServiceInstance(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance) error {
	plan, err := c.getServicePlan(ctx, cfServiceInstance)
	if err != nil {
		return err
	}

	broker, err := c.getBroker(ctx, plan)
	if err != nil {
		return err
	}

	deprovisionUrl, err := url.JoinPath(broker.Spec.URL, "v2", "service_instances", cfServiceInstance.Name)
	if err != nil {
		return fmt.Errorf("failed to construct service broker deprovision url: %w", err)
	}
	deprovisionUrl += "?" + url.Values{"accepts_incomplete": {"true"}}.Encode()

	respCode, resp, err := sendRequest(deprovisionUrl, http.MethodDelete, nil)
	if err != nil {
		return fmt.Errorf("deprovision request failed: %w", err)
	}

	if respCode > 299 {
		return fmt.Errorf("deprovision request returned non-OK status %d: %s", respCode, string(resp))
	}

	return nil
}

func (c *brokerClient) BindService(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) error {
	cfServiceInstance, err := c.getCFServiceInstance(ctx, cfServiceBinding)
	if err != nil {
		return err
	}
	plan, err := c.getServicePlan(ctx, cfServiceInstance)
	if err != nil {
		return err
	}

	offering, err := c.getServiceOffering(ctx, plan)
	if err != nil {
		return err
	}

	broker, err := c.getBroker(ctx, plan)
	if err != nil {
		return err
	}

	bindRequest := map[string]any{
		"service_id": offering.Spec.Broker_catalog.Id,
		"plan_id":    plan.Spec.Broker_catalog.Id,
	}

	bindUrl, err := url.JoinPath(broker.Spec.URL, "v2", "service_instances", cfServiceInstance.Name, "service_bindings", cfServiceBinding.Name)
	if err != nil {
		return fmt.Errorf("failed to construct service broker provision url: %w", err)
	}
	bindUrl += "?" + url.Values{"accepts_incomplete": {"true"}}.Encode()

	respCode, resp, err := sendRequest(bindUrl, http.MethodPut, bindRequest)
	if err != nil {
		return fmt.Errorf("bind request failed: %w", err)
	}

	if respCode > 299 {
		return fmt.Errorf("bind request returned non-OK status %d: %s", respCode, string(resp))
	}

	return nil
}

func (c *brokerClient) GetServiceBinding(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (ServiceBinding, error) {
	cfServiceInstance, err := c.getCFServiceInstance(ctx, cfServiceBinding)
	if err != nil {
		return ServiceBinding{}, err
	}
	plan, err := c.getServicePlan(ctx, cfServiceInstance)
	if err != nil {
		return ServiceBinding{}, err
	}

	offering, err := c.getServiceOffering(ctx, plan)
	if err != nil {
		return ServiceBinding{}, err
	}

	broker, err := c.getBroker(ctx, plan)
	if err != nil {
		return ServiceBinding{}, err
	}

	getRequest := map[string]any{
		"service_id": offering.Spec.Broker_catalog.Id,
		"plan_id":    plan.Spec.Broker_catalog.Id,
	}

	getUrl, err := url.JoinPath(broker.Spec.URL, "v2", "service_instances", cfServiceInstance.Name, "service_bindings", cfServiceBinding.Name)
	if err != nil {
		return ServiceBinding{}, fmt.Errorf("failed to construct service broker binding url: %w", err)
	}

	respCode, resp, err := sendRequest(getUrl, http.MethodGet, getRequest)
	if err != nil {
		return ServiceBinding{}, fmt.Errorf("get binding request failed: %w", err)
	}

	if respCode > 299 {
		return ServiceBinding{}, fmt.Errorf("get binding request returned non-OK status %d: %s", respCode, string(resp))
	}

	serviceBinding := ServiceBinding{}
	err = json.Unmarshal(resp, &serviceBinding)
	if err != nil {
		return ServiceBinding{}, fmt.Errorf("failed to unmarshal binding response: %w", err)
	}
	return serviceBinding, nil
}

func (c *brokerClient) GetServiceBindingLastOperation(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (LastOperation, error) {
	cfServiceInstance, err := c.getCFServiceInstance(ctx, cfServiceBinding)
	if err != nil {
		return LastOperation{}, err
	}

	plan, err := c.getServicePlan(ctx, cfServiceInstance)
	if err != nil {
		return LastOperation{}, err
	}

	broker, err := c.getBroker(ctx, plan)
	if err != nil {
		return LastOperation{}, err
	}

	stateUrl, err := url.JoinPath(broker.Spec.URL, "v2", "service_instances", cfServiceInstance.Name, "service_bindings", cfServiceBinding.Name, "last_operation")
	if err != nil {
		return LastOperation{}, fmt.Errorf("failed to construct service broker last operation url: %w", err)
	}

	respCode, resp, err := sendRequest(stateUrl, http.MethodGet, nil)
	if err != nil {
		return LastOperation{}, fmt.Errorf("last operation request failed: %w", err)
	}

	if respCode == http.StatusNotFound {
		return LastOperation{
			Exists: false,
		}, nil
	}

	respMap := map[string]string{}
	err = json.Unmarshal(resp, &respMap)
	if err != nil {
		return LastOperation{}, fmt.Errorf("failed to unmarshal last operation response: %w", err)
	}

	if respCode > 299 {
		return LastOperation{}, fmt.Errorf("last operation request failed: status code: %d: %v", respCode, respMap)
	}

	return LastOperation{
		Exists:      true,
		State:       respMap["state"],
		Description: respMap["description"],
	}, nil
}

func (c *brokerClient) UnbindService(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) error {
	cfServiceInstance, err := c.getCFServiceInstance(ctx, cfServiceBinding)
	if err != nil {
		return err
	}

	plan, err := c.getServicePlan(ctx, cfServiceInstance)
	if err != nil {
		return err
	}

	broker, err := c.getBroker(ctx, plan)
	if err != nil {
		return err
	}

	unbindUrl, err := url.JoinPath(broker.Spec.URL, "v2", "service_instances", cfServiceInstance.Name, "service_bindings", cfServiceBinding.Name)
	if err != nil {
		return fmt.Errorf("failed to construct service broker unbind url: %w", err)
	}
	unbindUrl += "?" + url.Values{"accepts_incomplete": {"true"}}.Encode()

	respCode, resp, err := sendRequest(unbindUrl, http.MethodDelete, nil)
	if err != nil {
		return fmt.Errorf("unbind request failed: %w", err)
	}

	if respCode > 299 {
		return fmt.Errorf("unbind request returned non-OK status %d: %s", respCode, string(resp))
	}

	return nil
}

func sendRequest(url string, method string, payload map[string]any) (int, []byte, error) {
	payloadReader, err := payloadToReader(payload)
	if err != nil {
		return 0, nil, fmt.Errorf("failed create payload reader: %w", err)
	}

	req, err := http.NewRequest(method, url, payloadReader)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create new HTTP request: %w", err)
	}

	// TODO: configure whether to trust insecure brokers
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
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

	return resp.StatusCode, respBody, nil
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

func (c *brokerClient) getServicePlan(ctx context.Context, cfServiceInstance *korifiv1alpha1.CFServiceInstance) (*korifiv1alpha1.CFServicePlan, error) {
	plan := &korifiv1alpha1.CFServicePlan{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: c.rootNamespace,
			Name:      cfServiceInstance.Spec.ServicePlanGUID,
		},
	}

	err := c.k8sClient.Get(ctx, client.ObjectKeyFromObject(plan), plan)
	if err != nil {
		return nil, fmt.Errorf("failed to get plan. rootNs: %q; planGUID %q", c.rootNamespace, cfServiceInstance.Spec.ServicePlanGUID)
	}

	return plan, nil
}

func (c *brokerClient) getServiceOffering(ctx context.Context, plan *korifiv1alpha1.CFServicePlan) (*korifiv1alpha1.CFServiceOffering, error) {
	offering := &korifiv1alpha1.CFServiceOffering{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: c.rootNamespace,
			Name:      plan.Labels[korifiv1alpha1.RelServiceOfferingLabel],
		},
	}

	err := c.k8sClient.Get(ctx, client.ObjectKeyFromObject(offering), offering)
	if err != nil {
		return nil, err
	}

	return offering, nil
}

func (c *brokerClient) getBroker(ctx context.Context, plan *korifiv1alpha1.CFServicePlan) (*korifiv1alpha1.CFServiceBroker, error) {
	brokerName, ok := plan.Labels[korifiv1alpha1.RelServiceBrokerLabel]
	if !ok {
		return nil, fmt.Errorf("plan %q has no broker guid label set", plan.Name)
	}

	broker := &korifiv1alpha1.CFServiceBroker{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: c.rootNamespace,
			Name:      brokerName,
		},
	}

	err := c.k8sClient.Get(ctx, client.ObjectKeyFromObject(broker), broker)
	if err != nil {
		return nil, fmt.Errorf("failed to get broker: %w", err)
	}

	return broker, nil
}

func (c *brokerClient) getCFServiceInstance(ctx context.Context, cfServiceBinding *korifiv1alpha1.CFServiceBinding) (*korifiv1alpha1.CFServiceInstance, error) {
	cfServiceInstance := &korifiv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cfServiceBinding.Namespace,
			Name:      cfServiceBinding.Spec.Service.Name,
		},
	}

	err := c.k8sClient.Get(ctx, client.ObjectKeyFromObject(cfServiceInstance), cfServiceInstance)
	if err != nil {
		return nil, fmt.Errorf("failed to get CFServiceInstance for binding: %w", err)
	}

	return cfServiceInstance, nil
}
