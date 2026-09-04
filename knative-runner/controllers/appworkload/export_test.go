package appworkload

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Test exports for white-box coverage of unexported helpers.

var (
	RevisionName         = revisionName
	SpecHash             = specHash
	FilterAppWorkloads   = filterAppWorkloads
	SanitizeName         = sanitizeName
	TruncateString       = truncateString
	KnativeServiceName   = knativeServiceName
	MarshaledPodSpec     = marshaledPodSpec
	BuildKnativeService  = buildKnativeService
	MarshalJSON          = &marshalJSON
	UnmarshalJSON        = &unmarshalJSON
	UnstructuredFromJSON = &unstructuredFromJSON
	MarshalSpecJSON      = &marshalSpecJSON
	SetNestedMap         = &setNestedMap
)

func NewTestAppWorkloadReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	workloadsToKsvc WorkloadToKnativeServiceConverter,
	log logr.Logger,
) *AppWorkloadReconciler {
	return &AppWorkloadReconciler{
		k8sClient:       c,
		scheme:          scheme,
		workloadsToKsvc: workloadsToKsvc,
		log:             log,
	}
}

func (r *AppWorkloadReconciler) EnqueueAppWorkloadRequests(ctx context.Context, o client.Object) []reconcile.Request {
	return r.enqueueAppWorkloadRequests(ctx, o)
}

func (r *AppWorkloadReconciler) DeleteLeftoverStatefulSets(ctx context.Context, appWorkload *korifiv1alpha1.AppWorkload) error {
	return r.deleteLeftoverStatefulSets(ctx, appWorkload)
}

func (r *AppWorkloadReconciler) CountReadyPods(ctx context.Context, appWorkload *korifiv1alpha1.AppWorkload) (int32, error) {
	return r.countReadyPods(ctx, appWorkload)
}

func (r *AppWorkloadReconciler) Finalize(ctx context.Context, appWorkload *korifiv1alpha1.AppWorkload) (reconcile.Result, error) {
	return r.finalize(ctx, appWorkload)
}
