package reconciler

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	"code.cloudfoundry.org/lager"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	eiriniControllerSource = "eirini-controller"
	crashEventType         = "Warning"
	labelInstanceIndex     = "korifi.cloudfoundry.org/instance-index"
	action                 = "crashing"
	lrpKind                = "LRP"
	statefulSetKind        = "StatefulSet"
)

//counterfeiter:generate . CrashEventGenerator

type CrashEventGenerator interface {
	Generate(context.Context, *corev1.Pod, lager.Logger) *CrashEvent
}

type CrashEvent struct {
	ProcessGUID    string
	Reason         string
	Instance       string
	Index          int
	ExitCode       int
	CrashCount     int
	CrashTimestamp int64
}

type PodCrash struct {
	logger              lager.Logger
	crashEventGenerator CrashEventGenerator
	client              client.Client
}

func NewPodCrash(logger lager.Logger, client client.Client, crashEventGenerator CrashEventGenerator) *PodCrash {
	return &PodCrash{
		logger:              logger,
		client:              client,
		crashEventGenerator: crashEventGenerator,
	}
}

func (r PodCrash) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := r.logger.Session("crash-event-reconciler", lager.Data{"namespace": request.Namespace, "name": request.Name})

	pod := &corev1.Pod{}
	if err := r.client.Get(ctx, request.NamespacedName, pod); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Error("pod does not exist", err)

			return reconcile.Result{}, nil
		}

		logger.Error("failed to get pod", err)

		return reconcile.Result{}, errors.Wrap(err, "failed to get pod")
	}

	crashEvent := r.crashEventGenerator.Generate(ctx, pod, logger)
	if crashEvent == nil {
		return reconcile.Result{}, nil
	}

	if r.eventAlreadyEmitted(crashEvent, pod) {
		return reconcile.Result{}, nil
	}

	statefulSetRef, err := r.getOwner(pod, statefulSetKind)
	if err != nil {
		logger.Debug("pod-without-statefulset-owner")

		return reconcile.Result{}, nil //nolint:nilerr
	}

	statefulSet := &appsv1.StatefulSet{}
	err = r.client.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: statefulSetRef.Name}, statefulSet)

	if err != nil {
		logger.Error("failed-to-get-stateful-set", err)

		return reconcile.Result{}, errors.Wrap(err, "failed to get stateful set")
	}

	lrpRef, err := r.getOwner(statefulSet, lrpKind)
	if err != nil {
		logger.Debug("statefulset-without-lrp-owner", lager.Data{"statefulset-name": statefulSet.Name})

		return reconcile.Result{}, nil //nolint:nilerr
	}

	kubeEvent, err := r.getByInstanceAndReason(ctx, request.Namespace, lrpRef, crashEvent.Index, failureReason(crashEvent))
	if err != nil {
		logger.Error("failed-to-get-existing-event", err)

		return reconcile.Result{}, errors.Wrap(err, "failed to get existing event")
	}

	err = r.setEvent(ctx, logger, kubeEvent, lrpRef, crashEvent, request.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}

	r.setCrashTimestampOnPod(ctx, logger, pod, crashEvent)

	return reconcile.Result{}, nil
}

func (r PodCrash) getByInstanceAndReason(ctx context.Context, namespace string, ownerRef metav1.OwnerReference, instanceIndex int, reason string) (*corev1.Event, error) {
	kubeEvents := &corev1.EventList{}

	err := r.client.List(ctx, kubeEvents,
		client.MatchingLabels{
			labelInstanceIndex: strconv.Itoa(instanceIndex),
		},
		client.InNamespace(namespace),
		client.MatchingFields{
			IndexEventInvolvedObjectName: ownerRef.Name,
		},
		client.MatchingFields{
			IndexEventInvolvedObjectKind: ownerRef.Kind,
		},
		client.MatchingFields{
			IndexEventReason: reason,
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list events")
	}

	if len(kubeEvents.Items) == 1 {
		return &kubeEvents.Items[0], nil
	}

	return nil, nil // nolint:nilnil // this is a private function, no need to create a custom not found error
}

func (r PodCrash) setEvent(ctx context.Context, logger lager.Logger, kubeEvent *corev1.Event, lrpRef metav1.OwnerReference,
	crashEvent *CrashEvent, namespace string) error {
	if kubeEvent != nil {
		return r.updateEvent(ctx, logger, kubeEvent, crashEvent, namespace)
	}

	return r.createEvent(ctx, logger, lrpRef, crashEvent, namespace)
}

func (r PodCrash) eventAlreadyEmitted(crashEvent *CrashEvent, pod *corev1.Pod) bool {
	return strconv.FormatInt(crashEvent.CrashTimestamp, 10) == pod.Annotations[stset.AnnotationLastReportedLRPCrash] // nolint:gomnd
}

func (r PodCrash) setCrashTimestampOnPod(ctx context.Context, logger lager.Logger, pod *corev1.Pod, crashEvent *CrashEvent) {
	newPod := pod.DeepCopy()
	if newPod.Annotations == nil {
		newPod.Annotations = map[string]string{}
	}

	newPod.Annotations[stset.AnnotationLastReportedLRPCrash] = strconv.FormatInt(crashEvent.CrashTimestamp, 10) // nolint:gomnd

	if err := r.client.Patch(ctx, newPod, client.MergeFrom(pod)); err != nil {
		logger.Error("failed-to-set-last-crash-time-on-pod", err)
	}
}

func (r PodCrash) createEvent(ctx context.Context, logger lager.Logger, ownerRef metav1.OwnerReference, crashEvent *CrashEvent, namespace string) error {
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    namespace,
			GenerateName: fmt.Sprintf("%s-", crashEvent.Instance),
			Labels: map[string]string{
				labelInstanceIndex: strconv.Itoa(crashEvent.Index),
			},
			Annotations: map[string]string{
				stset.AnnotationProcessGUID: crashEvent.ProcessGUID,
			},
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       ownerRef.Kind,
			Name:       ownerRef.Name,
			UID:        ownerRef.UID,
			APIVersion: ownerRef.APIVersion,
			Namespace:  namespace,
			FieldPath:  "spec.containers{opi}",
		},
		Reason:  failureReason(crashEvent),
		Message: fmt.Sprintf("Container terminated with exit code: %d", crashEvent.ExitCode),
		Source: corev1.EventSource{
			Component: eiriniControllerSource,
		},
		FirstTimestamp:      metav1.NewTime(time.Unix(crashEvent.CrashTimestamp, 0)),
		LastTimestamp:       metav1.NewTime(time.Unix(crashEvent.CrashTimestamp, 0)),
		EventTime:           metav1.NewMicroTime(time.Now()),
		Count:               1,
		Type:                crashEventType,
		ReportingController: eiriniControllerSource,
		Action:              action,
		ReportingInstance:   "controller-id",
	}
	if err := r.client.Create(ctx, event); err != nil {
		logger.Error("failed-to-create-event", err)

		return errors.Wrap(err, "failed to create event")
	}

	logger.Debug("event-created", lager.Data{"name": event.Name, "namespace": event.Namespace})

	return nil
}

func (r PodCrash) updateEvent(ctx context.Context, logger lager.Logger, kubeEvent *corev1.Event, crashEvent *CrashEvent, namespace string) error {
	kubeEvent.Count++
	kubeEvent.LastTimestamp = metav1.NewTime(time.Unix(crashEvent.CrashTimestamp, 0))

	if err := r.client.Update(ctx, kubeEvent); err != nil {
		logger.Error("failed-to-update-event", err)

		return errors.Wrap(err, "failed to update event")
	}

	logger.Debug("event-updated", lager.Data{"name": kubeEvent.Name, "namespace": kubeEvent.Namespace})

	return nil
}

func (r PodCrash) getOwner(obj metav1.Object, kind string) (metav1.OwnerReference, error) {
	for _, ref := range obj.GetOwnerReferences() {
		if ref.Kind == kind {
			return ref, nil
		}
	}

	return metav1.OwnerReference{}, fmt.Errorf("no owner of kind %q", kind)
}

func failureReason(crashEvent *CrashEvent) string {
	return fmt.Sprintf("Container: %s", crashEvent.Reason)
}
