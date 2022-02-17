package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RouteResourceType = "Route"
)

//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfroutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfroutes/status,verbs=get

type RouteRepo struct {
	privilegedClient  client.Client
	userClientFactory UserK8sClientFactory
}

func NewRouteRepo(privilegedClient client.Client, userClientFactory UserK8sClientFactory) *RouteRepo {
	return &RouteRepo{
		privilegedClient:  privilegedClient,
		userClientFactory: userClientFactory,
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
	Host        string
	Path        string
	SpaceGUID   string
	DomainGUID  string
	Labels      map[string]string
	Annotations map[string]string
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
			Name:        uuid.NewString(),
			Namespace:   m.SpaceGUID,
			Labels:      m.Labels,
			Annotations: m.Annotations,
		},
		Spec: networkingv1alpha1.CFRouteSpec{
			Host:     m.Host,
			Path:     m.Path,
			Protocol: "http",
			DomainRef: v1.LocalObjectReference{
				Name: m.DomainGUID,
			},
		},
	}
}

func (f *RouteRepo) GetRoute(ctx context.Context, authInfo authorization.Info, routeGUID string) (RouteRecord, error) {
	// TODO: Could look up namespace from guid => namespace cache to do Get
	cfRouteList := &networkingv1alpha1.CFRouteList{}
	err := f.privilegedClient.List(ctx, cfRouteList, client.MatchingFields{"metadata.name": routeGUID})
	if err != nil {
		return RouteRecord{}, err
	}

	toReturn, err := returnRoute(cfRouteList.Items)
	return toReturn, err
}

func (f *RouteRepo) ListRoutes(ctx context.Context, authInfo authorization.Info, message ListRoutesMessage) ([]RouteRecord, error) {
	cfRouteList := &networkingv1alpha1.CFRouteList{}
	err := f.privilegedClient.List(ctx, cfRouteList)
	if err != nil {
		return []RouteRecord{}, err
	}

	filtered := applyFilter(cfRouteList.Items, message)

	return returnRouteList(filtered), nil
}

func applyFilter(routes []networkingv1alpha1.CFRoute, message ListRoutesMessage) []networkingv1alpha1.CFRoute {
	// TODO: refactor this to be less repetitive
	var appFiltered []networkingv1alpha1.CFRoute

	if len(message.AppGUIDs) > 0 {
		for _, route := range routes {
			for _, destination := range route.Spec.Destinations {
				for _, appGUID := range message.AppGUIDs {
					if destination.AppRef.Name == appGUID {
						appFiltered = append(appFiltered, route)
						break
					}
				}
			}
		}
	} else {
		appFiltered = routes
	}

	var spaceFiltered []networkingv1alpha1.CFRoute

	if len(message.SpaceGUIDs) > 0 {
		for _, route := range appFiltered {
			for _, spaceGUID := range message.SpaceGUIDs {
				if route.Namespace == spaceGUID {
					spaceFiltered = append(spaceFiltered, route)
					break
				}
			}
		}
	} else {
		spaceFiltered = appFiltered
	}

	var domainFiltered []networkingv1alpha1.CFRoute

	if len(message.DomainGUIDs) > 0 {
		for _, route := range spaceFiltered {
			for _, domainGUID := range message.DomainGUIDs {
				if route.Spec.DomainRef.Name == domainGUID {
					domainFiltered = append(domainFiltered, route)
					break
				}
			}
		}
	} else {
		domainFiltered = spaceFiltered
	}

	var hostFiltered []networkingv1alpha1.CFRoute

	if len(message.Hosts) > 0 {
		for _, route := range domainFiltered {
			for _, host := range message.Hosts {
				if route.Spec.Host == host {
					hostFiltered = append(hostFiltered, route)
					break
				}
			}
		}
	} else {
		hostFiltered = domainFiltered
	}

	var pathFiltered []networkingv1alpha1.CFRoute

	if len(message.Paths) > 0 {
		for _, route := range hostFiltered {
			for _, path := range message.Paths {
				if route.Spec.Path == path {
					pathFiltered = append(pathFiltered, route)
					break
				}
			}
		}
	} else {
		pathFiltered = hostFiltered
	}

	return pathFiltered
}

func (f *RouteRepo) ListRoutesForApp(ctx context.Context, authInfo authorization.Info, appGUID string, spaceGUID string) ([]RouteRecord, error) {
	cfRouteList := &networkingv1alpha1.CFRouteList{}
	err := f.privilegedClient.List(ctx, cfRouteList, client.InNamespace(spaceGUID))
	if err != nil {
		return []RouteRecord{}, err
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

func returnRoute(routeList []networkingv1alpha1.CFRoute) (RouteRecord, error) {
	if len(routeList) == 0 {
		return RouteRecord{}, NewNotFoundError(RouteResourceType, nil)
	}

	if len(routeList) > 1 {
		return RouteRecord{}, errors.New("duplicate route GUID exists")
	}

	return cfRouteToRouteRecord(routeList[0]), nil
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
	err := f.privilegedClient.Create(ctx, &cfRoute)
	if err != nil {
		return RouteRecord{}, err
	}

	return cfRouteToRouteRecord(cfRoute), err
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
	if err == nil {
		return nil
	}

	if apierrors.IsForbidden(err) {
		return NewForbiddenError(RouteResourceType, err)
	}

	return err
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

	err := f.privilegedClient.Patch(ctx, cfRoute, client.MergeFrom(baseCFRoute))
	if err != nil { // untested
		return RouteRecord{}, fmt.Errorf("err in client.Patch: %w", err)
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
