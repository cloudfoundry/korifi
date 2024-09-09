package repositories

import (
	"context"
	"fmt"
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/google/uuid"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RouteResourceType = "Route"
	RoutePrefix       = "cf-route-"
)

type RouteRepo struct {
	namespaceRetriever   NamespaceRetriever
	userClientFactory    authorization.UserK8sClientFactory
	namespacePermissions *authorization.NamespacePermissions
}

func NewRouteRepo(namespaceRetriever NamespaceRetriever, userClientFactory authorization.UserK8sClientFactory, authPerms *authorization.NamespacePermissions) *RouteRepo {
	return &RouteRepo{
		namespaceRetriever:   namespaceRetriever,
		userClientFactory:    userClientFactory,
		namespacePermissions: authPerms,
	}
}

type DestinationRecord struct {
	GUID        string
	AppGUID     string
	ProcessType string
	Port        *int32
	Protocol    *string
	// Weight intentionally omitted as experimental features
}

type RouteRecord struct {
	GUID         string
	SpaceGUID    string
	Domain       DomainRecord
	Host         string
	Path         string
	Protocol     string
	Destinations []DestinationRecord
	Labels       map[string]string
	Annotations  map[string]string
	CreatedAt    time.Time
	UpdatedAt    *time.Time
	DeletedAt    *time.Time
}

func (r RouteRecord) Relationships() map[string]string {
	return map[string]string{
		"space":  r.SpaceGUID,
		"domain": r.Domain.GUID,
	}
}

type DesiredDestination struct {
	AppGUID     string
	ProcessType string
	Port        *int32
	Protocol    *string
	// Weight intentionally omitted as experimental features
}

type AddDestinationsMessage struct {
	RouteGUID            string
	SpaceGUID            string
	ExistingDestinations []DestinationRecord
	NewDestinations      []DesiredDestination
}

type RemoveDestinationMessage struct {
	RouteGUID string
	SpaceGUID string
	GUID      string
}

func (m *RemoveDestinationMessage) matches(dest korifiv1alpha1.Destination) bool {
	return dest.GUID == m.GUID
}

type PatchRouteMetadataMessage struct {
	MetadataPatch
	RouteGUID string
	SpaceGUID string
}

type ListRoutesMessage struct {
	AppGUIDs    []string
	SpaceGUIDs  []string
	DomainGUIDs []string
	Hosts       []string
	Paths       []string
}

func (m *ListRoutesMessage) matches(r korifiv1alpha1.CFRoute) bool {
	return tools.EmptyOrContains(m.DomainGUIDs, r.Spec.DomainRef.Name) &&
		tools.EmptyOrContains(m.Hosts, r.Spec.Host) &&
		tools.EmptyOrContains(m.Paths, r.Spec.Path) &&
		m.matchesApp(r)
}

func (m *ListRoutesMessage) matchesNamespace(ns string) bool {
	return tools.EmptyOrContains(m.SpaceGUIDs, ns)
}

func (m *ListRoutesMessage) matchesApp(r korifiv1alpha1.CFRoute) bool {
	if len(m.AppGUIDs) == 0 {
		return true
	}

	return len(itx.FromSlice(r.Spec.Destinations).Filter(func(d korifiv1alpha1.Destination) bool {
		return slices.Contains(m.AppGUIDs, d.AppRef.Name)
	}).Collect()) > 0
}

type CreateRouteMessage struct {
	Host            string
	Path            string
	SpaceGUID       string
	DomainGUID      string
	DomainName      string
	DomainNamespace string
	Labels          map[string]string
	Annotations     map[string]string
}

type DeleteRouteMessage struct {
	GUID      string
	SpaceGUID string
}

func (m CreateRouteMessage) toCFRoute() korifiv1alpha1.CFRoute {
	return korifiv1alpha1.CFRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       Kind,
			APIVersion: APIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        RoutePrefix + uuid.NewString(),
			Namespace:   m.SpaceGUID,
			Labels:      m.Labels,
			Annotations: m.Annotations,
		},
		Spec: korifiv1alpha1.CFRouteSpec{
			Host:     m.Host,
			Path:     m.Path,
			Protocol: "http",
			DomainRef: v1.ObjectReference{
				Name:      m.DomainGUID,
				Namespace: m.DomainNamespace,
			},
		},
	}
}

func (r *RouteRepo) GetRoute(ctx context.Context, authInfo authorization.Info, routeGUID string) (RouteRecord, error) {
	ns, err := r.namespaceRetriever.NamespaceFor(ctx, routeGUID, RouteResourceType)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to get namespace for route: %w", err)
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var route korifiv1alpha1.CFRoute
	err = userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: routeGUID}, &route)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to get route %q: %w", routeGUID, apierrors.FromK8sError(err, RouteResourceType))
	}

	return cfRouteToRouteRecord(route), nil
}

func (r *RouteRepo) ListRoutes(ctx context.Context, authInfo authorization.Info, message ListRoutesMessage) ([]RouteRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []RouteRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	authorizedSpaceNamespaces, err := authorizedSpaceNamespaces(ctx, authInfo, r.namespacePermissions)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}

	nsList := authorizedSpaceNamespaces.Filter(message.matchesNamespace).Collect()
	routes := []korifiv1alpha1.CFRoute{}
	for _, ns := range nsList {
		cfRouteList := &korifiv1alpha1.CFRouteList{}
		err := userClient.List(ctx, cfRouteList, client.InNamespace(ns))
		if k8serrors.IsForbidden(err) {
			continue
		}
		if err != nil {
			return []RouteRecord{}, fmt.Errorf("failed to list routes namespace %s: %w", ns, apierrors.FromK8sError(err, RouteResourceType))
		}
		routes = append(routes, cfRouteList.Items...)
	}

	filteredRoutes := itx.FromSlice(routes).Filter(message.matches)
	return slices.Collect(it.Map(filteredRoutes, cfRouteToRouteRecord)), nil
}

func cfRouteToRouteRecord(cfRoute korifiv1alpha1.CFRoute) RouteRecord {
	return RouteRecord{
		GUID:      cfRoute.Name,
		SpaceGUID: cfRoute.Namespace,
		Domain: DomainRecord{
			GUID: cfRoute.Spec.DomainRef.Name,
		},
		Host:         cfRoute.Spec.Host,
		Path:         cfRoute.Spec.Path,
		Protocol:     "http", // TODO: Create a mutating webhook to set this default on the CFRoute
		Destinations: cfRouteDestinationsToDestinationRecords(cfRoute),
		CreatedAt:    cfRoute.CreationTimestamp.Time,
		UpdatedAt:    getLastUpdatedTime(&cfRoute),
		DeletedAt:    golangTime(cfRoute.DeletionTimestamp),
		Labels:       cfRoute.Labels,
		Annotations:  cfRoute.Annotations,
	}
}

func cfRouteDestinationsToDestinationRecords(cfRoute korifiv1alpha1.CFRoute) []DestinationRecord {
	return slices.Collect(it.Map(slices.Values(cfRoute.Spec.Destinations), func(specDestination korifiv1alpha1.Destination) DestinationRecord {
		record := DestinationRecord{
			GUID:        specDestination.GUID,
			AppGUID:     specDestination.AppRef.Name,
			ProcessType: specDestination.ProcessType,
			Port:        specDestination.Port,
			Protocol:    specDestination.Protocol,
		}

		if record.Port == nil {
			effectiveDestination := findEffectiveDestination(specDestination.GUID, cfRoute.Status.Destinations)
			if effectiveDestination != nil {
				record.Protocol = effectiveDestination.Protocol
				record.Port = effectiveDestination.Port
			}
		}

		return record
	}))
}

func (r *RouteRepo) ListRoutesForApp(ctx context.Context, authInfo authorization.Info, appGUID string, spaceGUID string) ([]RouteRecord, error) {
	return r.ListRoutes(ctx, authInfo, ListRoutesMessage{
		AppGUIDs:   []string{appGUID},
		SpaceGUIDs: []string{spaceGUID},
	})
}

func findEffectiveDestination(destGUID string, effectiveDestinations []korifiv1alpha1.Destination) *korifiv1alpha1.Destination {
	for _, dest := range effectiveDestinations {
		if dest.GUID == destGUID {
			return &dest
		}
	}

	return nil
}

func (r *RouteRepo) CreateRoute(ctx context.Context, authInfo authorization.Info, message CreateRouteMessage) (RouteRecord, error) {
	cfRoute := message.toCFRoute()
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	err = userClient.Create(ctx, &cfRoute)
	if err != nil {
		return RouteRecord{}, apierrors.FromK8sError(err, RouteResourceType)
	}

	return cfRouteToRouteRecord(cfRoute), nil
}

func (r *RouteRepo) DeleteRoute(ctx context.Context, authInfo authorization.Info, message DeleteRouteMessage) error {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}
	err = userClient.Delete(ctx, &korifiv1alpha1.CFRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: message.SpaceGUID,
		},
	})

	return apierrors.FromK8sError(err, RouteResourceType)
}

func (r *RouteRepo) GetOrCreateRoute(ctx context.Context, authInfo authorization.Info, message CreateRouteMessage) (RouteRecord, error) {
	existingRecord, err := r.fetchRouteByFields(ctx, authInfo, message)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("GetOrCreateRoute: %w", err)
	}

	if existingRecord != nil {
		return *existingRecord, nil
	}

	return r.CreateRoute(ctx, authInfo, message)
}

func (r *RouteRepo) AddDestinationsToRoute(ctx context.Context, authInfo authorization.Info, message AddDestinationsMessage) (RouteRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfRoute := &korifiv1alpha1.CFRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.RouteGUID,
			Namespace: message.SpaceGUID,
		},
	}
	err = k8s.PatchResource(ctx, userClient, cfRoute, func() {
		cfRoute.Spec.Destinations = mergeDestinations(message.ExistingDestinations, message.NewDestinations)
	})
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to add destination to route %q: %w", message.RouteGUID, apierrors.FromK8sError(err, RouteResourceType))
	}

	return cfRouteToRouteRecord(*cfRoute), err
}

func (r *RouteRepo) RemoveDestinationFromRoute(ctx context.Context, authInfo authorization.Info, message RemoveDestinationMessage) (RouteRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfRoute := &korifiv1alpha1.CFRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.RouteGUID,
			Namespace: message.SpaceGUID,
		},
	}
	err = userClient.Get(ctx, client.ObjectKeyFromObject(cfRoute), cfRoute)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to get route: %w", apierrors.FromK8sError(err, RouteResourceType))
	}

	updatedDestinations := itx.FromSlice(cfRoute.Spec.Destinations).Exclude(message.matches).Collect()
	if len(updatedDestinations) == len(cfRoute.Spec.Destinations) {
		return RouteRecord{}, apierrors.NewUnprocessableEntityError(nil, "Unable to unmap route from destination. Ensure the route has a destination with this guid.")
	}

	err = k8s.PatchResource(ctx, userClient, cfRoute, func() {
		cfRoute.Spec.Destinations = updatedDestinations
	})
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to remove destination from route %q: %w", message.RouteGUID, apierrors.FromK8sError(err, RouteResourceType))
	}

	return cfRouteToRouteRecord(*cfRoute), err
}

func mergeDestinations(existingDestinations []DestinationRecord, desiredDestinations []DesiredDestination) []korifiv1alpha1.Destination {
	destinations := destinationRecordsToCFDestinations(existingDestinations)

	for _, desired := range desiredDestinations {
		if contains(destinations, desired) {
			continue
		}

		destinations = append(destinations, destinationMessageToDestination(desired))
	}

	return destinations
}

func destinationMessageToDestination(m DesiredDestination) korifiv1alpha1.Destination {
	return korifiv1alpha1.Destination{
		GUID: uuid.NewString(),
		Port: m.Port,
		AppRef: v1.LocalObjectReference{
			Name: m.AppGUID,
		},
		ProcessType: m.ProcessType,
		Protocol:    m.Protocol,
	}
}

func contains(existingDestinations []korifiv1alpha1.Destination, desired DesiredDestination) bool {
	_, ok := itx.FromSlice(existingDestinations).Find(func(dest korifiv1alpha1.Destination) bool {
		return desired.AppGUID == dest.AppRef.Name &&
			desired.ProcessType == dest.ProcessType &&
			equal(desired.Port, dest.Port) &&
			equal(desired.Protocol, dest.Protocol)
	})

	return ok
}

func equal[T comparable](v1, v2 *T) bool {
	if v1 == nil && v2 == nil {
		return true
	}

	if v1 != nil && v2 != nil {
		return *v1 == *v2
	}

	return false
}

func (r *RouteRepo) fetchRouteByFields(ctx context.Context, authInfo authorization.Info, message CreateRouteMessage) (*RouteRecord, error) {
	matches, err := r.ListRoutes(ctx, authInfo, ListRoutesMessage{
		SpaceGUIDs:  []string{message.SpaceGUID},
		DomainGUIDs: []string{message.DomainGUID},
		Hosts:       []string{message.Host},
		Paths:       []string{message.Path},
	})
	if err != nil {
		return nil, err
	}

	if len(matches) == 0 {
		return nil, nil
	}

	return &matches[0], nil
}

func destinationRecordsToCFDestinations(destinationRecords []DestinationRecord) []korifiv1alpha1.Destination {
	return slices.Collect(it.Map(itx.FromSlice(destinationRecords), func(destinationRecord DestinationRecord) korifiv1alpha1.Destination {
		return korifiv1alpha1.Destination{
			GUID: destinationRecord.GUID,
			Port: destinationRecord.Port,
			AppRef: v1.LocalObjectReference{
				Name: destinationRecord.AppGUID,
			},
			ProcessType: destinationRecord.ProcessType,
			Protocol:    destinationRecord.Protocol,
		}
	}))
}

func (r *RouteRepo) PatchRouteMetadata(ctx context.Context, authInfo authorization.Info, message PatchRouteMetadataMessage) (RouteRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	route := &korifiv1alpha1.CFRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: message.SpaceGUID,
			Name:      message.RouteGUID,
		},
	}
	err = userClient.Get(ctx, client.ObjectKeyFromObject(route), route)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to get route: %w", apierrors.FromK8sError(err, RouteResourceType))
	}

	err = k8s.PatchResource(ctx, userClient, route, func() {
		message.Apply(route)
	})
	if err != nil {
		return RouteRecord{}, apierrors.FromK8sError(err, RouteResourceType)
	}

	return cfRouteToRouteRecord(*route), nil
}

func (r *RouteRepo) GetDeletedAt(ctx context.Context, authInfo authorization.Info, routeGUID string) (*time.Time, error) {
	route, err := r.GetRoute(ctx, authInfo, routeGUID)
	return route.DeletedAt, err
}
