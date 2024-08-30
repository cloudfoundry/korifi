package controllers

import (
	"context"
	"strings"

	"code.cloudfoundry.org/korifi/tools/image"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	kpackv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const KpackBuildFinalizer string = "korifi.cloudfoundry.org/kpackBuild"

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
) *KpackBuildController {
	return &KpackBuildController{
		log:                    log,
		k8sClient:              k8sClient,
		imageDeleter:           imageDeleter,
		registryServiceAccount: registryServiceAccount,
	}
}

func (c *KpackBuildController) SetupWithManager(mgr manager.Manager) error {
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
		WithEventFilter(labelSelector).
		Complete(c)
}

//+kubebuilder:rbac:groups=kpack.io,resources=builds,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups=kpack.io,resources=builds/status,verbs=get;patch
//+kubebuilder:rbac:groups=kpack.io,resources=builds/finalizers,verbs=get;patch

func (c *KpackBuildController) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	log := c.log.WithName("KpackBuild").
		WithValues("namespace", req.Namespace).
		WithValues("name", req.Name).
		WithValues("logID", uuid.NewString())

	kpackBuild := &kpackv1alpha2.Build{}
	err := c.k8sClient.Get(ctx, req.NamespacedName, kpackBuild)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Info("unable to fetch kpack build", "reason", err)
		return ctrl.Result{}, err
	}

	if !kpackBuild.GetDeletionTimestamp().IsZero() {
		if !controllerutil.ContainsFinalizer(kpackBuild, KpackBuildFinalizer) {
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

			err = c.imageDeleter.Delete(ctx, image.Creds{
				Namespace:          kpackBuild.Namespace,
				ServiceAccountName: c.registryServiceAccount,
			}, kpackBuild.Status.LatestImage, tagsToDelete...)
			if err != nil {
				log.Info("failed to delete droplet image", "reason", err)
			}
		}

		err = k8s.Patch(ctx, c.k8sClient, kpackBuild, func() {
			if controllerutil.RemoveFinalizer(kpackBuild, KpackBuildFinalizer) {
				log.V(1).Info("finalizer removed")
			}
		})
		if err != nil {
			log.Info("unable to remove finalizer from kpack build", "reason", err)
			return ctrl.Result{}, err

		}
	}

	return ctrl.Result{}, nil
}
