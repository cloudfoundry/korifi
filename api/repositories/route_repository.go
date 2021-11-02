package repositories

import (
	"context"
	"errors"
	"fmt"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfroutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfroutes/status,verbs=get

type RouteRepo struct{}

type DestinationRecord struct {
	GUID        string
	AppGUID     string
	ProcessType string
	Port        int
	// Weight and Protocol intentionally omitted as experimental features
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

type RouteAddDestinationsMessage struct {
	Route        RouteRecord
	Destinations []DestinationRecord
}

func (f *RouteRepo) FetchRoute(ctx context.Context, client client.Client, routeGUID string) (RouteRecord, error) {
	// TODO: Could look up namespace from guid => namespace cache to do Get
	cfRouteList := &networkingv1alpha1.CFRouteList{}
	err := client.List(ctx, cfRouteList)

	if err != nil {
		return RouteRecord{}, err
	}

	routeList := cfRouteList.Items
	filteredRouteList := f.filterByRouteName(routeList, routeGUID)

	toReturn, err := f.returnRoute(filteredRouteList)
	return toReturn, err
}

func (f *RouteRepo) FetchRouteList(ctx context.Context, client client.Client) ([]RouteRecord, error) {
	cfRouteList := &networkingv1alpha1.CFRouteList{}
	err := client.List(ctx, cfRouteList)

	if err != nil {
		return []RouteRecord{}, err
	}

	return f.returnRouteList(cfRouteList.Items), nil
}

func (f *RouteRepo) FetchRoutesForApp(ctx context.Context, k8sClient client.Client, appGUID string, spaceGUID string) ([]RouteRecord, error) {
	cfRouteList := &networkingv1alpha1.CFRouteList{}
	err := k8sClient.List(ctx, cfRouteList, client.InNamespace(spaceGUID))
	if err != nil {
		return []RouteRecord{}, err
	}
	filteredRouteList := f.filterByAppDestination(cfRouteList.Items, appGUID)

	return f.returnRouteList(filteredRouteList), nil
}

func (r RouteRecord) UpdateDomainRef(d DomainRecord) RouteRecord {
	r.Domain = d

	return r
}

func (f *RouteRepo) filterByRouteName(routeList []networkingv1alpha1.CFRoute, name string) []networkingv1alpha1.CFRoute {
	var filtered []networkingv1alpha1.CFRoute

	for i, route := range routeList {
		if route.Name == name {
			filtered = append(filtered, routeList[i])
		}
	}

	return filtered
}

func (f *RouteRepo) filterByAppDestination(routeList []networkingv1alpha1.CFRoute, appGUID string) []networkingv1alpha1.CFRoute {
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

func (f *RouteRepo) returnRoute(routeList []networkingv1alpha1.CFRoute) (RouteRecord, error) {
	if len(routeList) == 0 {
		return RouteRecord{}, NotFoundError{}
	}

	if len(routeList) > 1 {
		return RouteRecord{}, errors.New("duplicate route GUID exists")
	}

	return cfRouteToRouteRecord(routeList[0]), nil
}

func (f *RouteRepo) returnRouteList(routeList []networkingv1alpha1.CFRoute) []RouteRecord {
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
	}
}

func (f *RouteRepo) CreateRoute(ctx context.Context, client client.Client, routeRecord RouteRecord) (RouteRecord, error) {
	cfRoute := f.routeRecordToCFRoute(routeRecord)
	err := client.Create(ctx, &cfRoute)
	if err != nil {
		return RouteRecord{}, err
	}

	return cfRouteToRouteRecord(cfRoute), err
}

func (f *RouteRepo) routeRecordToCFRoute(routeRecord RouteRecord) networkingv1alpha1.CFRoute {
	return networkingv1alpha1.CFRoute{
		TypeMeta: metav1.TypeMeta{
			Kind:       Kind,
			APIVersion: APIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        routeRecord.GUID,
			Namespace:   routeRecord.SpaceGUID,
			Labels:      routeRecord.Labels,
			Annotations: routeRecord.Annotations,
		},
		Spec: networkingv1alpha1.CFRouteSpec{
			Host: routeRecord.Host,
			Path: routeRecord.Path,
			DomainRef: v1.LocalObjectReference{
				Name: routeRecord.Domain.GUID,
			},
		},
	}
}

func (f *RouteRepo) AddDestinationsToRoute(ctx context.Context, c client.Client, message RouteAddDestinationsMessage) (RouteRecord, error) {
	baseCFRoute := &networkingv1alpha1.CFRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.Route.GUID,
			Namespace: message.Route.SpaceGUID,
		},
		Spec: networkingv1alpha1.CFRouteSpec{
			Destinations: destinationRecordsToCFDestinations(message.Route.Destinations),
		},
	}

	cfRoute := baseCFRoute.DeepCopy()
	cfRoute.Spec.Destinations = append(cfRoute.Spec.Destinations, destinationRecordsToCFDestinations(message.Destinations)...)

	err := c.Patch(ctx, cfRoute, client.MergeFrom(baseCFRoute))
	if err != nil { // untested
		return RouteRecord{}, fmt.Errorf("err in client.Patch: %w", err)
	}

	return cfRouteToRouteRecord(*cfRoute), err
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
		})
	}

	return destinations
}
