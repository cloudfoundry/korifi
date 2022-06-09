package wiring

import (
	"context"

	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	eirinievent "code.cloudfoundry.org/korifi/statefulset-runner/k8s/event"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/reconciler"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	"code.cloudfoundry.org/lager"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func PodCrashReconciler(logger lager.Logger, manager manager.Manager, config eirinictrl.ControllerConfig) error {
	logger = logger.Session("pod-crash-reconciler")

	podCrashReconciler := createPodCrashReconciler(logger, config.WorkloadsNamespace, manager.GetClient())

	err := addEventIndexes(manager)
	if err != nil {
		return err
	}

	predicates := []predicate.Predicate{reconciler.NewSourceTypeUpdatePredicate(stset.AppSourceType)}
	err = builder.
		ControllerManagedBy(manager).
		For(&corev1.Pod{}, builder.WithPredicates(predicates...)).
		Complete(podCrashReconciler)

	return errors.Wrapf(err, "Failed to build Pod Crash reconciler")
}

func addEventIndexes(manager manager.Manager) error {
	err := manager.GetFieldIndexer().IndexField(context.Background(), &corev1.Event{}, reconciler.IndexEventInvolvedObjectName, getEventInvolvedObjectName())
	if err != nil {
		return errors.Wrapf(err, "Failed to create index %q", reconciler.IndexEventInvolvedObjectName)
	}

	err = manager.GetFieldIndexer().IndexField(context.Background(), &corev1.Event{}, reconciler.IndexEventInvolvedObjectKind, getEventInvolvedObjectKind())
	if err != nil {
		return errors.Wrapf(err, "Failed to create index %q", reconciler.IndexEventInvolvedObjectKind)
	}

	err = manager.GetFieldIndexer().IndexField(context.Background(), &corev1.Event{}, reconciler.IndexEventReason, getEventReason())

	return errors.Wrapf(err, "Failed to create index %q", reconciler.IndexEventReason)
}

func getEventInvolvedObjectName() client.IndexerFunc {
	return createEventOwnerIndexerFunc(func(event *corev1.Event) string {
		return event.InvolvedObject.Name
	})
}

func getEventInvolvedObjectKind() client.IndexerFunc {
	return createEventOwnerIndexerFunc(func(event *corev1.Event) string {
		return event.InvolvedObject.Kind
	})
}

func getEventReason() client.IndexerFunc {
	return createEventOwnerIndexerFunc(func(event *corev1.Event) string {
		return event.Reason
	})
}

func createEventOwnerIndexerFunc(getEventAttribute func(*corev1.Event) string) client.IndexerFunc {
	return func(rawObj client.Object) []string {
		event, _ := rawObj.(*corev1.Event)

		return []string{getEventAttribute(event)}
	}
}

func createPodCrashReconciler(logger lager.Logger, workloadsNamespace string, controllerClient client.Client) *reconciler.PodCrash {
	crashEventGenerator := eirinievent.NewDefaultCrashEventGenerator(controllerClient)

	return reconciler.NewPodCrash(logger, controllerClient, crashEventGenerator)
}
