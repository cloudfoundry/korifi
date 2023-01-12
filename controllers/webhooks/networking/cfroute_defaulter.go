package networking

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// +kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-cfroute,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfroutes,verbs=create;update,versions=v1alpha1,name=mcfroute.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

type CFRouteDefaulter struct {
	logger logr.Logger
	client client.Client
}

var _ webhook.CustomDefaulter = &CFRouteDefaulter{}

func NewCFRouteDefaulter() *CFRouteDefaulter {
	return &CFRouteDefaulter{
		logger: logf.Log.WithName("CFRouteDefaulter"),
	}
}

func (d *CFRouteDefaulter) SetupWebhookWithManager(mgr ctrl.Manager) error {
	d.client = mgr.GetClient()

	return ctrl.NewWebhookManagedBy(mgr).
		For(&korifiv1alpha1.CFRoute{}).
		WithDefaulter(d).
		Complete()
}

func (d *CFRouteDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	cfRoute, ok := obj.(*korifiv1alpha1.CFRoute)
	if !ok {
		return fmt.Errorf("object is not a cfroute %T", obj)
	}

	log := d.logger.WithValues("name", cfRoute.Name, "namespace", cfRoute.Namespace)
	log.Info("Default")

	routeLabels := cfRoute.GetLabels()

	if routeLabels == nil {
		routeLabels = make(map[string]string)
	}

	routeLabels[korifiv1alpha1.CFDomainGUIDLabelKey] = cfRoute.Spec.DomainRef.Name
	routeLabels[korifiv1alpha1.CFRouteGUIDLabelKey] = cfRoute.Name
	cfRoute.SetLabels(routeLabels)

	domain, err := d.fetchDomain(ctx, cfRoute)
	if err != nil {
		log.Error(err, "failed to get domain for route")
		return fmt.Errorf("failed to fetch domain: %w", err)
	}

	if err := controllerutil.SetOwnerReference(domain, cfRoute, d.client.Scheme()); err != nil {
		log.Error(err, "failed to set owner ref")
		return fmt.Errorf("failed to set owner ref: %w", err)
	}

	return nil
}

func (d *CFRouteDefaulter) fetchDomain(ctx context.Context, route *korifiv1alpha1.CFRoute) (*korifiv1alpha1.CFDomain, error) {
	domain := &korifiv1alpha1.CFDomain{}
	err := d.client.Get(ctx, types.NamespacedName{Name: route.Spec.DomainRef.Name, Namespace: route.Spec.DomainRef.Namespace}, domain)

	return domain, err
}
