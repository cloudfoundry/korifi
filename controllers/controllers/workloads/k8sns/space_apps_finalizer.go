package k8sns

import (
	"context"
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SpaceAppsFinalizer struct {
	client             client.Client
	appDeletionTimeout int64
}

func NewSpaceAppsFinalizer(
	client client.Client,
	appDeletionTimeout int64,
) *SpaceAppsFinalizer {
	return &SpaceAppsFinalizer{
		client:             client,
		appDeletionTimeout: appDeletionTimeout,
	}
}

func (f *SpaceAppsFinalizer) Finalize(ctx context.Context, cfSpace *korifiv1alpha1.CFSpace) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx).WithName("finalize-contained-apps")

	duration := time.Since(cfSpace.GetDeletionTimestamp().Time)
	log.V(1).Info("finalizing contained apps", "duration", duration.Seconds())

	spaceNamespace := new(corev1.Namespace)
	err := f.client.Get(ctx, types.NamespacedName{Name: cfSpace.GetName()}, spaceNamespace)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.V(1).Info("namespace not found")

			return ctrl.Result{}, nil
		}

		log.Info("failed to get namespace", "reason", err)
		return ctrl.Result{}, err
	}

	log.V(1).Info("namespace found")

	if !spaceNamespace.GetDeletionTimestamp().IsZero() {
		log.V(1).Info("namespace already being deleted")
		return ctrl.Result{}, nil
	}

	appList := korifiv1alpha1.CFAppList{}
	err = f.client.List(ctx, &appList, client.InNamespace(cfSpace.GetName()))
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to list CFApps: %w", err)
	}

	timedOut := duration >= time.Duration(f.appDeletionTimeout)*time.Second

	if len(appList.Items) == 0 {
		log.V(1).Info("all CFApps deleted")
		return ctrl.Result{}, nil
	}

	if timedOut {
		log.Info("timed out deleting CFApps")
		return ctrl.Result{}, nil
	}

	log.V(1).Info("deleting all CFApps in namespace")
	err = f.client.DeleteAllOf(
		ctx,
		new(korifiv1alpha1.CFApp),
		client.InNamespace(cfSpace.GetName()),
		client.PropagationPolicy(metav1.DeletePropagationForeground),
	)
	if err != nil {
		log.Info("failed to delete CFApps", "reason", err)
	}

	log.V(1).Info("requeuing waiting for CFApp deletion")

	return ctrl.Result{RequeueAfter: 500 * time.Millisecond}, nil
}
