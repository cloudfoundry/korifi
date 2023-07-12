package repositories

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

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
	Port        int
	Protocol    string
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

type AddDestinationsToRouteMessage struct {
	RouteGUID            string
	SpaceGUID            string
	ExistingDestinations []DestinationRecord
	NewDestinations      []DestinationMessage
}

type RemoveDestinationFromRouteMessage struct {
	RouteGUID       string
	SpaceGUID       string
	DestinationGuid string
}

type DestinationMessage struct {
	AppGUID     string
	ProcessType string
	Port        int
	Protocol    string
	// Weight intentionally omitted as experimental features
}

type PatchRouteMetadataMessage struct {
	MetadataPatch
	RouteGUID string
	SpaceGUID string
}

func (m DestinationMessage) toCFDestination() korifiv1alpha1.Destination {
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

type ListRoutesMessage struct {
	AppGUIDs    []string
	SpaceGUIDs  []string
	DomainGUIDs []string
	Hosts       []string
	Paths       []string
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
	nsList, err := r.namespacePermissions.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []RouteRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	preds := []func(korifiv1alpha1.CFRoute) bool{
		SetPredicate(message.DomainGUIDs, func(s korifiv1alpha1.CFRoute) string { return s.Spec.DomainRef.Name }),
		SetPredicate(message.Hosts, func(s korifiv1alpha1.CFRoute) string { return s.Spec.Host }),
		SetPredicate(message.Paths, func(s korifiv1alpha1.CFRoute) string { return s.Spec.Path }),
	}
	if len(message.AppGUIDs) > 0 {
		appGUIDsSet := NewSet(message.AppGUIDs...)
		preds = append(preds, func(r korifiv1alpha1.CFRoute) bool {
			for _, dest := range r.Spec.Destinations {
				if appGUIDsSet.Includes(dest.AppRef.Name) {
					return true
				}
			}
			return false
		})
	}

	filteredRoutes := []korifiv1alpha1.CFRoute{}
	spaceGUIDSet := NewSet(message.SpaceGUIDs...)
	for ns := range nsList {
		if len(spaceGUIDSet) > 0 && !spaceGUIDSet.Includes(ns) {
			continue
		}

		cfRouteList := &korifiv1alpha1.CFRouteList{}
		err := userClient.List(ctx, cfRouteList, client.InNamespace(ns))
		if k8serrors.IsForbidden(err) {
			continue
		}
		if err != nil {
			return []RouteRecord{}, fmt.Errorf("failed to list routes namespace %s: %w", ns, apierrors.FromK8sError(err, RouteResourceType))
		}
		filteredRoutes = append(filteredRoutes, Filter(cfRouteList.Items, preds...)...)
	}

	return returnRouteList(filteredRoutes), nil
}

func (r *RouteRepo) ListRoutesForApp(ctx context.Context, authInfo authorization.Info, appGUID string, spaceGUID string) ([]RouteRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []RouteRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfRouteList := &korifiv1alpha1.CFRouteList{}
	err = userClient.List(ctx, cfRouteList, client.InNamespace(spaceGUID))
	if err != nil {
		return []RouteRecord{}, apierrors.FromK8sError(err, RouteResourceType)
	}
	filteredRouteList := filterByAppDestination(cfRouteList.Items, appGUID)

	return returnRouteList(filteredRouteList), nil
}

func filterByAppDestination(routeList []korifiv1alpha1.CFRoute, appGUID string) []korifiv1alpha1.CFRoute {
	var filtered []korifiv1alpha1.CFRoute

	for i, route := range routeList {
		if len(route.Spec.Destinations) == 0 {
			continue
		}
		for _, destination := range route.Spec.Destinations {
			if destination.AppRef.Name == appGUID {
				filtered = append(filtered, routeList[i])
				break
			}
		}
	}

	return filtered
}

func returnRouteList(routeList []korifiv1alpha1.CFRoute) []RouteRecord {
	routeRecords := make([]RouteRecord, 0, len(routeList))

	for _, route := range routeList {
		routeRecords = append(routeRecords, cfRouteToRouteRecord(route))
	}
	return routeRecords
}

func cfRouteToRouteRecord(cfRoute korifiv1alpha1.CFRoute) RouteRecord {
	destinations := []DestinationRecord{}
	for _, destination := range cfRoute.Spec.Destinations {
		destinations = append(destinations, cfRouteDestinationToDestination(destination))
	}
	return RouteRecord{
		GUID:      cfRoute.Name,
		SpaceGUID: cfRoute.Namespace,
		Domain: DomainRecord{
			GUID: cfRoute.Spec.DomainRef.Name,
		},
		Host:         cfRoute.Spec.Host,
		Path:         cfRoute.Spec.Path,
		Protocol:     "http", // TODO: Create a mutating webhook to set this default on the CFRoute
		Destinations: destinations,
		CreatedAt:    cfRoute.CreationTimestamp.Time,
		UpdatedAt:    getLastUpdatedTime(&cfRoute),
		DeletedAt:    golangTime(cfRoute.DeletionTimestamp),
		Labels:       cfRoute.Labels,
		Annotations:  cfRoute.Annotations,
	}
}

func cfRouteDestinationToDestination(cfRouteDestination korifiv1alpha1.Destination) DestinationRecord {
	return DestinationRecord{
		GUID:        cfRouteDestination.GUID,
		AppGUID:     cfRouteDestination.AppRef.Name,
		ProcessType: cfRouteDestination.ProcessType,
		Port:        cfRouteDestination.Port,
		Protocol:    cfRouteDestination.Protocol,
	}
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
	existingRecord, exists, err := r.fetchRouteByFields(ctx, authInfo, message)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("GetOrCreateRoute: %w", err)
	}

	if exists {
		return existingRecord, nil
	}

	return r.CreateRoute(ctx, authInfo, message)
}

func (r *RouteRepo) AddDestinationsToRoute(ctx context.Context, authInfo authorization.Info, message AddDestinationsToRouteMessage) (RouteRecord, error) {
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

func (r *RouteRepo) RemoveDestinationFromRoute(ctx context.Context, authInfo authorization.Info, message RemoveDestinationFromRouteMessage) (RouteRecord, error) {
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

	oldCfRoute := cfRoute.DeepCopy()

	updatedDestinations := []korifiv1alpha1.Destination{}
	for _, dest := range cfRoute.Spec.Destinations {
		if dest.GUID != message.DestinationGuid {
			updatedDestinations = append(updatedDestinations, dest)
		}
	}

	if len(updatedDestinations) == len(cfRoute.Spec.Destinations) {
		return RouteRecord{}, apierrors.NewUnprocessableEntityError(nil, "Unable to unmap route from destination. Ensure the route has a destination with this guid.")
	}
	cfRoute.Spec.Destinations = updatedDestinations

	err = userClient.Patch(ctx, cfRoute, client.MergeFrom(oldCfRoute))
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to remove destination from route %q: %w", message.RouteGUID, apierrors.FromK8sError(err, RouteResourceType))
	}

	return cfRouteToRouteRecord(*cfRoute), err
}

func mergeDestinations(existingDestinations []DestinationRecord, newDestinations []DestinationMessage) []korifiv1alpha1.Destination {
	result := destinationRecordsToCFDestinations(existingDestinations)

outer:
	for _, newDest := range newDestinations {
		for _, oldDest := range result {
			if newDest.AppGUID == oldDest.AppRef.Name &&
				newDest.ProcessType == oldDest.ProcessType &&
				newDest.Port == oldDest.Port &&
				newDest.Protocol == oldDest.Protocol {
				continue outer
			}
		}
		result = append(result, newDest.toCFDestination())
	}

	return result
}

func (r *RouteRepo) fetchRouteByFields(ctx context.Context, authInfo authorization.Info, message CreateRouteMessage) (RouteRecord, bool, error) {
	matches, err := r.ListRoutes(ctx, authInfo, ListRoutesMessage{
		SpaceGUIDs:  []string{message.SpaceGUID},
		DomainGUIDs: []string{message.DomainGUID},
		Hosts:       []string{message.Host},
		Paths:       []string{message.Path},
	})
	if err != nil {
		return RouteRecord{}, false, err
	}

	if len(matches) == 0 {
		return RouteRecord{}, false, nil
	}

	return matches[0], true, nil
}

func destinationRecordsToCFDestinations(destinationRecords []DestinationRecord) []korifiv1alpha1.Destination {
	var destinations []korifiv1alpha1.Destination
	for _, destinationRecord := range destinationRecords {
		destinations = append(destinations, korifiv1alpha1.Destination{
			GUID: destinationRecord.GUID,
			Port: destinationRecord.Port,
			AppRef: v1.LocalObjectReference{
				Name: destinationRecord.AppGUID,
			},
			ProcessType: destinationRecord.ProcessType,
			Protocol:    destinationRecord.Protocol,
		})
	}

	return destinations
}

func (r *RouteRepo) PatchRouteMetadata(ctx context.Context, authInfo authorization.Info, message PatchRouteMetadataMessage) (RouteRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	route := new(korifiv1alpha1.CFRoute)
	err = userClient.Get(ctx, client.ObjectKey{Namespace: message.SpaceGUID, Name: message.RouteGUID}, route)
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
