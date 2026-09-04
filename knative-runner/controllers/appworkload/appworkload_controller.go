package appworkload

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/knative-runner/controllers"
	"code.cloudfoundry.org/korifi/statefulset-runner/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var knativeServiceGVK = schema.GroupVersionKind{
	Group:   "serving.knative.dev",
	Version: "v1",
	Kind:    "Service",
}

// Overridable in tests to exercise error paths.
var (
	marshalSpecJSON = json.Marshal
	setNestedMap    = unstructured.SetNestedMap
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o ./fake -fake-name WorkloadToKnativeServiceConverter . WorkloadToKnativeServiceConverter
type WorkloadToKnativeServiceConverter interface {
	Convert(appWorkload *korifiv1alpha1.AppWorkload) (*unstructured.Unstructured, error)
}

type AppWorkloadReconciler struct {
	k8sClient       client.Client
	scheme          *runtime.Scheme
	workloadsToKsvc WorkloadToKnativeServiceConverter
	log             logr.Logger
}

func NewAppWorkloadReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	workloadsToKsvc WorkloadToKnativeServiceConverter,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.AppWorkload] {
	r := &AppWorkloadReconciler{
		k8sClient:       c,
		scheme:          scheme,
		workloadsToKsvc: workloadsToKsvc,
		log:             log,
	}
	return k8s.NewPatchingReconciler[korifiv1alpha1.AppWorkload](log, c, r)
}

func (r *AppWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	ksvc := &unstructured.Unstructured{}
	ksvc.SetGroupVersionKind(knativeServiceGVK)

	return ctrl.NewControllerManagedBy(mgr).
		Named("knative-appworkload").
		For(&korifiv1alpha1.AppWorkload{}).
		Watches(
			ksvc,
			handler.EnqueueRequestsFromMapFunc(r.enqueueAppWorkloadRequests),
		).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueAppWorkloadRequests),
		).
		WithEventFilter(predicate.NewPredicateFuncs(filterAppWorkloads))
}

func (r *AppWorkloadReconciler) enqueueAppWorkloadRequests(_ context.Context, o client.Object) []reconcile.Request {
	var requests []reconcile.Request
	if appWorkloadName, ok := o.GetLabels()[LabelAppWorkloadGUID]; ok {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      appWorkloadName,
				Namespace: o.GetNamespace(),
			},
		})
	}
	return requests
}

func filterAppWorkloads(object client.Object) bool {
	appWorkload, ok := object.(*korifiv1alpha1.AppWorkload)
	if !ok {
		return true
	}
	return appWorkload.Spec.RunnerName == controllers.AppWorkloadReconcilerName
}

func (r *AppWorkloadReconciler) ReconcileResource(ctx context.Context, appWorkload *korifiv1alpha1.AppWorkload) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	if appWorkload.Spec.RunnerName != controllers.AppWorkloadReconcilerName {
		return ctrl.Result{}, nil
	}

	appWorkload.Status.ObservedGeneration = appWorkload.Generation
	log.V(1).Info("set observed generation", "generation", appWorkload.Status.ObservedGeneration)

	if !appWorkload.GetDeletionTimestamp().IsZero() {
		return r.finalize(ctx, appWorkload)
	}

	if err := r.deleteLeftoverStatefulSets(ctx, appWorkload); err != nil {
		log.Info("error deleting leftover StatefulSets", "reason", err)
		return ctrl.Result{}, err
	}

	desired, err := r.workloadsToKsvc.Convert(appWorkload)
	if err != nil {
		log.Info("error when converting AppWorkload", "reason", err)
		return ctrl.Result{}, err
	}

	desiredSpec, found, err := unstructured.NestedMap(desired.Object, "spec")
	if err != nil {
		return ctrl.Result{}, err
	}
	if !found {
		return ctrl.Result{}, fmt.Errorf("converted knative service has no spec")
	}
	desiredHash, err := specHash(desiredSpec)
	if err != nil {
		return ctrl.Result{}, err
	}

	specToApply := runtime.DeepCopyJSON(desiredSpec)
	if revName := revisionName(desired.GetName(), desiredHash); revName != "" {
		if err = unstructured.SetNestedField(specToApply, revName, "template", "metadata", "name"); err != nil {
			return ctrl.Result{}, err
		}
	}

	created := &unstructured.Unstructured{}
	created.SetGroupVersionKind(knativeServiceGVK)
	created.SetName(desired.GetName())
	created.SetNamespace(desired.GetNamespace())

	_, err = controllerutil.CreateOrPatch(ctx, r.k8sClient, created, func() error {
		labels := created.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		liveHash := labels[LabelSpecHash]
		for k, v := range desired.GetLabels() {
			labels[k] = v
		}
		labels[LabelSpecHash] = desiredHash
		created.SetLabels(labels)

		anns := created.GetAnnotations()
		if anns == nil {
			anns = map[string]string{}
		}
		for k, v := range desired.GetAnnotations() {
			if len(k) >= 19 && k[:19] == "serving.knative.dev" {
				continue
			}
			anns[k] = v
		}
		created.SetAnnotations(anns)

		// Knative defaults (timeoutSeconds, probes, …) mutate spec after create.
		// Replacing spec every reconcile looks like a new revision and storms pods.
		// Persist the desired-spec hash as a label (annotations can be stripped).
		isCreate := created.GetUID() == ""
		if isCreate || liveHash != desiredHash {
			if setErr := setNestedMap(created.Object, specToApply, "spec"); setErr != nil {
				return setErr
			}
		}
		return controllerutil.SetControllerReference(appWorkload, created, r.scheme)
	})
	if err != nil {
		log.Info("error when creating or updating Knative Service", "reason", err)
		return ctrl.Result{}, err
	}

	readyReplicas, err := r.countReadyPods(ctx, appWorkload)
	if err != nil {
		log.Info("error when listing app pods", "reason", err)
		return ctrl.Result{}, err
	}
	appWorkload.Status.ActualInstances = readyReplicas
	state := korifiv1alpha1.InstanceStateDown
	if readyReplicas > 0 {
		state = korifiv1alpha1.InstanceStateRunning
	}
	appWorkload.Status.InstancesStatus = map[string]korifiv1alpha1.InstanceStatus{
		"0": {State: state},
	}

	return ctrl.Result{}, nil
}

const LabelSpecHash = "korifi.cloudfoundry.org/knative-spec-hash"

// revisionName is `{ksvc}-{hash}` truncated to the 63-char DNS limit. Knative
// requires the revision name to be prefixed with the Service name.
func revisionName(ksvcName, hash string) string {
	const maxLen = 63
	prefix := ksvcName + "-"
	if len(prefix) >= maxLen {
		return ""
	}
	remain := maxLen - len(prefix)
	if len(hash) > remain {
		hash = hash[:remain]
	}
	return prefix + hash
}

func specHash(spec map[string]any) (string, error) {
	raw, err := marshalSpecJSON(spec)
	if err != nil {
		return "", fmt.Errorf("marshal spec for hash: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])[:16], nil
}

func (r *AppWorkloadReconciler) deleteLeftoverStatefulSets(ctx context.Context, appWorkload *korifiv1alpha1.AppWorkload) error {
	stsList := &appsv1.StatefulSetList{}
	if err := r.k8sClient.List(ctx, stsList, client.InNamespace(appWorkload.Namespace), client.MatchingLabels{
		LabelAppWorkloadGUID: appWorkload.Name,
	}); err != nil {
		return err
	}
	for i := range stsList.Items {
		if err := r.k8sClient.Delete(ctx, &stsList.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (r *AppWorkloadReconciler) countReadyPods(ctx context.Context, appWorkload *korifiv1alpha1.AppWorkload) (int32, error) {
	podList := &corev1.PodList{}
	if err := r.k8sClient.List(ctx, podList, client.InNamespace(appWorkload.Namespace), client.MatchingLabels{
		LabelAppWorkloadGUID: appWorkload.Name,
	}); err != nil {
		return 0, err
	}

	var ready int32
	for _, pod := range podList.Items {
		if pod.DeletionTimestamp != nil {
			continue
		}
		for _, c := range pod.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
				ready++
				break
			}
		}
	}
	return ready, nil
}

func (r *AppWorkloadReconciler) finalize(ctx context.Context, appWorkload *korifiv1alpha1.AppWorkload) (ctrl.Result, error) {
	ksvcList := &unstructured.UnstructuredList{}
	ksvcList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   knativeServiceGVK.Group,
		Version: knativeServiceGVK.Version,
		Kind:    knativeServiceGVK.Kind + "List",
	})

	if err := r.k8sClient.List(ctx, ksvcList, client.InNamespace(appWorkload.Namespace), client.MatchingLabels{
		LabelAppWorkloadGUID: appWorkload.Name,
	}); err != nil {
		return ctrl.Result{}, err
	}

	for i := range ksvcList.Items {
		if err := r.k8sClient.Delete(ctx, &ksvcList.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	if err := r.k8sClient.List(ctx, ksvcList, client.InNamespace(appWorkload.Namespace), client.MatchingLabels{
		LabelAppWorkloadGUID: appWorkload.Name,
	}); err != nil {
		return ctrl.Result{}, err
	}

	if len(ksvcList.Items) == 0 {
		if controllerutil.RemoveFinalizer(appWorkload, finalizer.AppWorkloadFinalizerName) {
			r.log.V(1).Info("removing finalizer from AppWorkload", "appWorkload", appWorkload.Name)
		}
		return ctrl.Result{}, nil
	}

	appWorkload.Status.ActualInstances = int32(len(ksvcList.Items)) // #nosec G115 -- instance count fits int32
	return ctrl.Result{}, k8s.NewNotReadyError().
		WithMessage(fmt.Sprintf("%d knative services still present", len(ksvcList.Items))).
		WithReason("StillRunning").
		WithRequeue()
}
