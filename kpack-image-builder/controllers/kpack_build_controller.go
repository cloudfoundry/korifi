package controllers

import (
	"context"
	"strings"

	"code.cloudfoundry.org/korifi/tools/image"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	kpackv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const kpackBuildFinalizer string = "korifi.cloudfoundry.org/kpackBuild"

//counterfeiter:generate -o fake -fake-name ImageDeleter . ImageDeleter

type ImageDeleter interface {
	Delete(context.Context, image.Creds, string, ...string) error
}

type KpackBuildController struct {
	log                    logr.Logger
	k8sClient              client.Client
	imageDeleter           ImageDeleter
	registryServiceAccount string
}

func NewKpackBuildController(
	k8sClient client.Client,
	log logr.Logger,
	imageDeleter ImageDeleter,
	registryServiceAccount string,
) *k8s.PatchingReconciler[kpackv1alpha2.Build, *kpackv1alpha2.Build] {
	return k8s.NewPatchingReconciler[kpackv1alpha2.Build, *kpackv1alpha2.Build](log, k8sClient, KpackBuildController{
		log:                    log,
		k8sClient:              k8sClient,
		imageDeleter:           imageDeleter,
		registryServiceAccount: registryServiceAccount,
	})
}

func (c KpackBuildController) SetupWithManager(mgr manager.Manager) *ctrl.Builder {
	// ignoring error as this construction is not dynamic
	labelSelector, _ := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      BuildWorkloadLabelKey,
				Operator: metav1.LabelSelectorOpExists,
				Values:   []string{},
			},
		},
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&kpackv1alpha2.Build{}).
		WithEventFilter(labelSelector)
}

//+kubebuilder:rbac:groups=kpack.io,resources=builds,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups=kpack.io,resources=builds/status,verbs=get;patch
//+kubebuilder:rbac:groups=kpack.io,resources=builds/finalizers,verbs=get;patch

func (c KpackBuildController) ReconcileResource(ctx context.Context, kpackBuild *kpackv1alpha2.Build) (ctrl.Result, error) {
	log := c.log.WithValues("namespace", kpackBuild.Namespace, "name", kpackBuild.Name, "deletionTimestamp", kpackBuild.DeletionTimestamp)

	if !kpackBuild.GetDeletionTimestamp().IsZero() {
		if !controllerutil.ContainsFinalizer(kpackBuild, kpackBuildFinalizer) {
			return ctrl.Result{}, nil
		}

		if kpackBuild.Status.LatestImage != "" {
			tagsToDelete := []string{}
			for _, t := range kpackBuild.Spec.Tags {
				parts := strings.Split(t, ":")
				if len(parts) == 2 {
					tagsToDelete = append(tagsToDelete, parts[1])
				}
			}

			err := c.imageDeleter.Delete(ctx, image.Creds{
				Namespace:          kpackBuild.Namespace,
				ServiceAccountName: c.registryServiceAccount,
			}, kpackBuild.Status.LatestImage, tagsToDelete...)
			if err != nil {
				log.Info("failed to delete droplet image", "reason", err)
			}
		}

		if controllerutil.RemoveFinalizer(kpackBuild, kpackBuildFinalizer) {
			log.V(1).Info("finalizer removed")
		}

		return ctrl.Result{}, nil
	}

	err := k8s.AddFinalizer(ctx, log, c.k8sClient, kpackBuild, kpackBuildFinalizer)
	if err != nil {
		log.Error(err, "Error adding finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
