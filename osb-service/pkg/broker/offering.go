package broker

import "context"

// Offering is one CF marketplace service. Implement this and register it in
// newDefaultOfferings (see postgres.go). The broker is not tied to a cluster
// type; env/flags supply backing-store connection facts.
type Offering interface {
	Catalog() Service
	// Provision allocates the backing resource and returns bind credentials.
	Provision(id string, req ProvisionRequest) (instance, error)
	// Deprovision releases the backing resource. The store row is removed by BusinessLogic.
	Deprovision(inst instance) error
	Healthy(ctx context.Context) error
}

type instance struct {
	req         ProvisionRequest
	credentials map[string]any
}

type store interface {
	getInstance(id string) (instance, bool, error)
	putInstance(id string, inst instance) error
	deleteInstance(id string) (bool, error)
	getBinding(instanceID, bindingID string) (BindResponse, bool, error)
	putBinding(instanceID, bindingID string, resp BindResponse) error
	deleteBinding(instanceID, bindingID string) (bool, error)
	healthy(ctx context.Context) error
}
