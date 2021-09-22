package repositories

import (
	"context"
	"errors"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/networking/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfroutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.cloudfoundry.org,resources=cfroutes/status,verbs=get

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
	Labels       map[string]string
	Annotations  map[string]string
	CreatedAt    string
	UpdatedAt    string
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
		Protocol:     "http", // TODO: Create a mutating webhook to set this default on the CFRoute
		Destinations: []Destination{},
		CreatedAt:    "",
		UpdatedAt:    "",
	}
}

func (f *RouteRepo) CreateRoute(ctx context.Context, client client.Client, routeRecord RouteRecord) (RouteRecord, error) {
	return RouteRecord{}, nil
}
