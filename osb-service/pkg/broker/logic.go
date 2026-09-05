package broker

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"sort"
	"sync"
)

const MinimumAPIVersion = "2.17"

// Compatibility aliases for the first (postgres) offering. New offerings
// should export their own IDs; tests and HTTP fixtures still use these.
const (
	ServiceID = PostgresServiceID
	PlanID    = PostgresPlanID
)

// BusinessLogic is the OSB catalog + lifecycle. Offerings are marketplace
// services; the store is shared instance/binding metadata.
type BusinessLogic struct {
	async     bool
	offerings map[string]Offering
	sync.RWMutex
	store store
}

func NewBusinessLogic(o Options) (*BusinessLogic, error) {
	st, offerings, err := newDefaultOfferings(o)
	if err != nil {
		return nil, err
	}
	b := &BusinessLogic{
		async:     o.Async,
		offerings: map[string]Offering{},
		store:     st,
	}
	for _, off := range offerings {
		b.register(off)
	}
	return b, nil
}

func (b *BusinessLogic) register(o Offering) {
	b.offerings[o.Catalog().ID] = o
}

func (b *BusinessLogic) Catalog() CatalogResponse {
	services := make([]Service, 0, len(b.offerings))
	for _, o := range b.offerings {
		services = append(services, o.Catalog())
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
	return CatalogResponse{Services: services}
}

func (b *BusinessLogic) Healthy(ctx context.Context) error {
	if err := b.store.healthy(ctx); err != nil {
		return err
	}
	for _, o := range b.offerings {
		if err := o.Healthy(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (b *BusinessLogic) offeringFor(serviceID string) (Offering, error) {
	o, ok := b.offerings[serviceID]
	if !ok {
		return nil, BadRequest("unknown service_id")
	}
	return o, nil
}

func planKnown(o Offering, planID string) bool {
	for _, p := range o.Catalog().Plans {
		if p.ID == planID {
			return true
		}
	}
	return false
}

func (b *BusinessLogic) Provision(id string, req ProvisionRequest) (ProvisionResponse, int, error) {
	b.Lock()
	defer b.Unlock()
	existing, ok, err := b.store.getInstance(id)
	if err != nil {
		return ProvisionResponse{}, 0, err
	}
	if ok {
		if reflect.DeepEqual(existing.req, req) {
			return ProvisionResponse{}, http.StatusOK, nil
		}
		return ProvisionResponse{}, 0, Conflict("instance ID is already in use")
	}
	off, err := b.offeringFor(req.ServiceID)
	if err != nil {
		return ProvisionResponse{}, 0, err
	}
	if !planKnown(off, req.PlanID) {
		return ProvisionResponse{}, 0, BadRequest("unknown plan_id")
	}
	inst, err := off.Provision(id, req)
	if err != nil {
		return ProvisionResponse{}, 0, err
	}
	if err := b.store.putInstance(id, inst); err != nil {
		_ = off.Deprovision(inst)
		return ProvisionResponse{}, 0, err
	}
	if b.async && req.AcceptsIncomplete {
		return ProvisionResponse{Operation: "provision"}, http.StatusAccepted, nil
	}
	return ProvisionResponse{}, http.StatusCreated, nil
}

func (b *BusinessLogic) GetInstance(id string) (GetInstanceResponse, error) {
	b.RLock()
	defer b.RUnlock()
	i, ok, err := b.store.getInstance(id)
	if err != nil {
		return GetInstanceResponse{}, err
	}
	if !ok {
		return GetInstanceResponse{}, NotFound("service instance not found")
	}
	return GetInstanceResponse{ServiceID: i.req.ServiceID, PlanID: i.req.PlanID, Parameters: i.req.Parameters, Metadata: i.req.Context}, nil
}

func (b *BusinessLogic) Update(id string, req UpdateRequest) (UpdateResponse, int, error) {
	b.Lock()
	defer b.Unlock()
	i, ok, err := b.store.getInstance(id)
	if err != nil {
		return UpdateResponse{}, 0, err
	}
	if !ok {
		return UpdateResponse{}, 0, NotFound("service instance not found")
	}
	if req.ServiceID != "" && req.ServiceID != i.req.ServiceID {
		return UpdateResponse{}, 0, BadRequest("service_id cannot be changed")
	}
	if req.PlanID != "" {
		i.req.PlanID = req.PlanID
	}
	if req.Parameters != nil {
		i.req.Parameters = req.Parameters
	}
	if req.Context != nil {
		i.req.Context = req.Context
	}
	if err := b.store.putInstance(id, i); err != nil {
		return UpdateResponse{}, 0, err
	}
	if b.async && req.AcceptsIncomplete {
		return UpdateResponse{Operation: "update"}, http.StatusAccepted, nil
	}
	return UpdateResponse{}, http.StatusOK, nil
}

func (b *BusinessLogic) Deprovision(id string, acceptsIncomplete bool) (DeprovisionResponse, int, error) {
	b.Lock()
	defer b.Unlock()
	inst, ok, err := b.store.getInstance(id)
	if err != nil {
		return DeprovisionResponse{}, 0, err
	}
	if !ok {
		return DeprovisionResponse{}, 0, Gone("service instance not found")
	}
	if off, err := b.offeringFor(inst.req.ServiceID); err == nil {
		if err := off.Deprovision(inst); err != nil {
			return DeprovisionResponse{}, 0, err
		}
	}
	if _, err := b.store.deleteInstance(id); err != nil {
		return DeprovisionResponse{}, 0, err
	}
	if b.async && acceptsIncomplete {
		return DeprovisionResponse{Operation: "deprovision"}, http.StatusAccepted, nil
	}
	return DeprovisionResponse{}, http.StatusOK, nil
}

func (b *BusinessLogic) Bind(instanceID, bindingID string, req BindRequest) (BindResponse, int, error) {
	b.Lock()
	defer b.Unlock()
	inst, ok, err := b.store.getInstance(instanceID)
	if err != nil {
		return BindResponse{}, 0, err
	}
	if !ok {
		return BindResponse{}, 0, NotFound("service instance not found")
	}
	_ = req
	response := BindResponse{Credentials: inst.credentials}
	existing, found, err := b.store.getBinding(instanceID, bindingID)
	if err != nil {
		return BindResponse{}, 0, err
	}
	if found {
		if reflect.DeepEqual(existing, response) {
			return existing, http.StatusOK, nil
		}
		return BindResponse{}, 0, Conflict("binding ID is already in use")
	}
	if err := b.store.putBinding(instanceID, bindingID, response); err != nil {
		return BindResponse{}, 0, err
	}
	return response, http.StatusCreated, nil
}

func (b *BusinessLogic) GetBinding(instanceID, bindingID string) (BindResponse, error) {
	b.RLock()
	defer b.RUnlock()
	v, ok, err := b.store.getBinding(instanceID, bindingID)
	if err != nil {
		return BindResponse{}, err
	}
	if !ok {
		return BindResponse{}, NotFound("service binding not found")
	}
	return v, nil
}

func (b *BusinessLogic) Unbind(instanceID, bindingID string) error {
	b.Lock()
	defer b.Unlock()
	ok, err := b.store.deleteBinding(instanceID, bindingID)
	if err != nil {
		return err
	}
	if !ok {
		return Gone("service binding not found")
	}
	return nil
}

func (b *BusinessLogic) LastOperation(id string) (LastOperationResponse, error) {
	b.RLock()
	defer b.RUnlock()
	_, ok, err := b.store.getInstance(id)
	if err != nil {
		return LastOperationResponse{}, err
	}
	if !ok {
		return LastOperationResponse{}, NotFound("service instance not found")
	}
	return LastOperationResponse{State: "succeeded"}, nil
}

func boolPtr(v bool) *bool { return &v }

type APIError struct {
	Status                 int
	ErrorCode, Description string
}

func (e APIError) Error() string { return e.Description }
func BadRequest(s string) error  { return APIError{http.StatusBadRequest, "BadRequest", s} }
func NotFound(s string) error    { return APIError{http.StatusNotFound, "NotFound", s} }
func Gone(s string) error        { return APIError{http.StatusGone, "Gone", s} }
func Conflict(s string) error    { return APIError{http.StatusConflict, "Conflict", s} }
func AsAPIError(err error) APIError {
	var e APIError
	if errors.As(err, &e) {
		return e
	}
	return APIError{http.StatusInternalServerError, "InternalServerError", err.Error()}
}
