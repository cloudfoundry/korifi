package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	"code.cloudfoundry.org/cf-k8s-controllers/controllers/webhooks"

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

//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfroutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfroutes/status,verbs=get

type RouteRepo struct {
	namespaceRetriever   NamespaceRetriever
	userClientFactory    UserK8sClientFactory
	namespacePermissions NamespacePermissions
}

func NewRouteRepo(namespaceRetriever NamespaceRetriever, userClientFactory UserK8sClientFactory, authPerms NamespacePermissions) *RouteRepo {
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
	CreatedAt    string
	UpdatedAt    string
}

type AddDestinationsToRouteMessage struct {
	RouteGUID            string
	SpaceGUID            string
	ExistingDestinations []DestinationRecord
	NewDestinations      []DestinationMessage
}

type DestinationMessage struct {
	AppGUID     string
	ProcessType string
	Port        int
	Protocol    string
	// Weight intentionally omitted as experimental features
}

func (m DestinationMessage) toCFDestination() networkingv1alpha1.Destination {
	return networkingv1alpha1.Destination{
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

func (m CreateRouteMessage) toCFRoute() networkingv1alpha1.CFRoute {
	return networkingv1alpha1.CFRoute{
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
		Spec: networkingv1alpha1.CFRouteSpec{
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

func (f *RouteRepo) GetRoute(ctx context.Context, authInfo authorization.Info, routeGUID string) (RouteRecord, error) {
	ns, err := f.namespaceRetriever.NamespaceFor(ctx, routeGUID, RouteResourceType)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to get namespace for route: %w", err)
	}

	userClient, err := f.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var route networkingv1alpha1.CFRoute
	err = userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: routeGUID}, &route)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to get route %q: %w", routeGUID, apierrors.FromK8sError(err, RouteResourceType))
	}

	return cfRouteToRouteRecord(route), nil
}

func (f *RouteRepo) ListRoutes(ctx context.Context, authInfo authorization.Info, message ListRoutesMessage) ([]RouteRecord, error) {
	nsList, err := f.namespacePermissions.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}

	userClient, err := f.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []RouteRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	filteredRoutes := []networkingv1alpha1.CFRoute{}
	for ns := range nsList {
		cfRouteList := &networkingv1alpha1.CFRouteList{}
		err := userClient.List(ctx, cfRouteList, client.InNamespace(ns))
		if k8serrors.IsForbidden(err) {
			continue
		}
		if err != nil {
			return []RouteRecord{}, fmt.Errorf("failed to list routes namespace %s: %w", ns, apierrors.FromK8sError(err, RouteResourceType))
		}
		filteredRoutes = append(filteredRoutes, applyRouteListFilter(cfRouteList.Items, message)...)
	}

	return returnRouteList(filteredRoutes), nil
}

func applyRouteListFilter(routes []networkingv1alpha1.CFRoute, message ListRoutesMessage) []networkingv1alpha1.CFRoute {
	if len(message.AppGUIDs) == 0 &&
		len(message.SpaceGUIDs) == 0 &&
		len(message.DomainGUIDs) == 0 &&
		len(message.Hosts) == 0 &&
		len(message.Paths) == 0 {
		return routes
	}

	var filtered []networkingv1alpha1.CFRoute
	for _, route := range routes {
		if matchesFilter(route.Namespace, message.SpaceGUIDs) &&
			matchesFilter(route.Spec.DomainRef.Name, message.DomainGUIDs) &&
			matchesFilter(route.Spec.Host, message.Hosts) &&
			matchesFilter(route.Spec.Path, message.Paths) {
			filtered = append(filtered, route)
		}
	}

	if len(message.AppGUIDs) == 0 {
		return filtered
	}

	var appFiltered []networkingv1alpha1.CFRoute

	for _, route := range filtered {
		for _, destination := range route.Spec.Destinations {
			for _, appGUID := range message.AppGUIDs {
				if destination.AppRef.Name == appGUID {
					appFiltered = append(appFiltered, route)
					break
				}
			}
		}
	}

	return appFiltered
}

func (f *RouteRepo) ListRoutesForApp(ctx context.Context, authInfo authorization.Info, appGUID string, spaceGUID string) ([]RouteRecord, error) {
	userClient, err := f.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []RouteRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfRouteList := &networkingv1alpha1.CFRouteList{}
	err = userClient.List(ctx, cfRouteList, client.InNamespace(spaceGUID))
	if err != nil {
		return []RouteRecord{}, apierrors.FromK8sError(err, RouteResourceType)
	}
	filteredRouteList := filterByAppDestination(cfRouteList.Items, appGUID)

	return returnRouteList(filteredRouteList), nil
}

func (r RouteRecord) UpdateDomainRef(d DomainRecord) RouteRecord {
	r.Domain = d

	return r
}

func filterByAppDestination(routeList []networkingv1alpha1.CFRoute, appGUID string) []networkingv1alpha1.CFRoute {
	var filtered []networkingv1alpha1.CFRoute

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

func returnRouteList(routeList []networkingv1alpha1.CFRoute) []RouteRecord {
	routeRecords := make([]RouteRecord, 0, len(routeList))

	for _, route := range routeList {
		routeRecords = append(routeRecords, cfRouteToRouteRecord(route))
	}
	return routeRecords
}

func cfRouteToRouteRecord(cfRoute networkingv1alpha1.CFRoute) RouteRecord {
	destinations := []DestinationRecord{}
	for _, destination := range cfRoute.Spec.Destinations {
		destinations = append(destinations, cfRouteDestinationToDestination(destination))
	}
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfRoute.ObjectMeta)
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
		CreatedAt:    cfRoute.CreationTimestamp.UTC().Format(TimestampFormat),
		UpdatedAt:    updatedAtTime,
	}
}

func cfRouteDestinationToDestination(cfRouteDestination networkingv1alpha1.Destination) DestinationRecord {
	return DestinationRecord{
		GUID:        cfRouteDestination.GUID,
		AppGUID:     cfRouteDestination.AppRef.Name,
		ProcessType: cfRouteDestination.ProcessType,
		Port:        cfRouteDestination.Port,
		Protocol:    cfRouteDestination.Protocol,
	}
}

func (f *RouteRepo) CreateRoute(ctx context.Context, authInfo authorization.Info, message CreateRouteMessage) (RouteRecord, error) {
	cfRoute := message.toCFRoute()
	userClient, err := f.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	err = userClient.Create(ctx, &cfRoute)
	if err != nil {
		if webhooks.IsValidationError(err) {
			return RouteRecord{}, apierrors.NewUnprocessableEntityError(err, webhooks.GetErrorMessage(err))
		}
		return RouteRecord{}, apierrors.FromK8sError(err, RouteResourceType)
	}

	return cfRouteToRouteRecord(cfRoute), nil
}

func (f *RouteRepo) DeleteRoute(ctx context.Context, authInfo authorization.Info, message DeleteRouteMessage) error {
	userClient, err := f.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}
	err = userClient.Delete(ctx, &networkingv1alpha1.CFRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: message.SpaceGUID,
		},
	})

	return apierrors.FromK8sError(err, RouteResourceType)
}

func (f *RouteRepo) GetOrCreateRoute(ctx context.Context, authInfo authorization.Info, message CreateRouteMessage) (RouteRecord, error) {
	existingRecord, exists, err := f.fetchRouteByFields(ctx, authInfo, message)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("GetOrCreateRoute: %w", err)
	}

	if exists {
		return existingRecord, nil
	}

	return f.CreateRoute(ctx, authInfo, message)
}

func (f *RouteRepo) AddDestinationsToRoute(ctx context.Context, authInfo authorization.Info, message AddDestinationsToRouteMessage) (RouteRecord, error) {
	baseCFRoute := &networkingv1alpha1.CFRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.RouteGUID,
			Namespace: message.SpaceGUID,
		},
	}

	cfRoute := baseCFRoute.DeepCopy()
	cfRoute.Spec.Destinations = mergeDestinations(message.ExistingDestinations, message.NewDestinations)

	userClient, err := f.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return RouteRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	err = userClient.Patch(ctx, cfRoute, client.MergeFrom(baseCFRoute))
	if err != nil {
		if webhooks.HasErrorCode(err, webhooks.RouteDestinationNotInSpace) {
			return RouteRecord{}, apierrors.NewUnprocessableEntityError(err, "Routes cannot be mapped to destinations in different spaces.")
		}
		return RouteRecord{}, fmt.Errorf("failed to add destination to route %q: %w", message.RouteGUID, apierrors.FromK8sError(err, RouteResourceType))
	}

	return cfRouteToRouteRecord(*cfRoute), err
}

func mergeDestinations(existingDestinations []DestinationRecord, newDestinations []DestinationMessage) []networkingv1alpha1.Destination {
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

func (f *RouteRepo) fetchRouteByFields(ctx context.Context, authInfo authorization.Info, message CreateRouteMessage) (RouteRecord, bool, error) {
	matches, err := f.ListRoutes(ctx, authInfo, ListRoutesMessage{
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

func destinationRecordsToCFDestinations(destinationRecords []DestinationRecord) []networkingv1alpha1.Destination {
	var destinations []networkingv1alpha1.Destination
	for _, destinationRecord := range destinationRecords {
		destinations = append(destinations, networkingv1alpha1.Destination{
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
