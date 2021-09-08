package repositories

import (
	"context"
	"errors"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type RouteRepo struct{}

type Destination struct {
	GUID        string
	AppGUID     string
	ProcessType string
	Port        int
	// Weight and Protocol intentionally omitted as experimental features
}

type RouteRecord struct {
	GUID         string
	SpaceGUID    string
	DomainRef    DomainRecord
	Host         string
	Path         string
	Protocol     string
	Destinations []Destination
	CreatedAt    string
	UpdatedAt    string
}

// TODO: Make a general ConfigureClient function / config and client generating package
func (f *RouteRepo) ConfigureClient(config *rest.Config) (client.Client, error) {
	routeClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, err
	}
	return routeClient, nil
}

func (f *RouteRepo) FetchRoute(client client.Client, routeGUID string) (RouteRecord, error) {
	// TODO: Could look up namespace from guid => namespace cache to do Get
	cfRouteList := &networkingv1alpha1.CFRouteList{}
	err := client.List(context.Background(), cfRouteList)

	if err != nil {
		return RouteRecord{}, err
	}

	routeList := cfRouteList.Items
	filteredRouteList := f.filterByRouteName(routeList, routeGUID)

	return f.returnRoute(filteredRouteList)
}

func (r RouteRecord) UpdateDomainRef(d DomainRecord) RouteRecord {
	r.DomainRef = d

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

func (f *RouteRepo) returnRoute(routeList []networkingv1alpha1.CFRoute) (RouteRecord, error) {
	if len(routeList) == 0 {
		return RouteRecord{}, NotFoundError{Err: errors.New("not found")}
	}

	if len(routeList) > 1 {
		return RouteRecord{}, errors.New("duplicate route GUID exists")
	}

	return cfRouteToRouteRecord(routeList[0]), nil
}

func cfRouteToRouteRecord(cfRoute networkingv1alpha1.CFRoute) RouteRecord {
	return RouteRecord{
		GUID:      cfRoute.Name,
		SpaceGUID: cfRoute.Namespace,
		DomainRef: DomainRecord{
			GUID: cfRoute.Spec.DomainRef.Name,
		},
		Host:         cfRoute.Spec.Host,
		Path:         cfRoute.Spec.Path,
		Protocol:     "http",
		Destinations: []Destination{},
		CreatedAt:    "",
		UpdatedAt:    "",
	}
}
