package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	kpackv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type KpackImageController struct {
	log       logr.Logger
	k8sClient client.Client
}

func NewKpackImageController(
	k8sClient client.Client,
	log logr.Logger,
) *KpackImageController {
	return &KpackImageController{
		log:       log,
		k8sClient: k8sClient,
	}
}

//+kubebuilder:rbac:groups=kpack.io,resources=images,verbs=get;list;watch

func (c *KpackImageController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := c.log.WithValues("namespace", req.Namespace, "name", req.Name)

	kpackImage := &kpackv1alpha2.Image{}
	err := c.k8sClient.Get(ctx, req.NamespacedName, kpackImage)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Info(fmt.Sprintf("unable to fetch %T", kpackImage), "reason", err)
		return ctrl.Result{}, err
	}

	if kpackImage.Status.LatestImage == "" {
		return ctrl.Result{}, nil
	}

	if !isUnattendedBuild(kpackImage) {
		return ctrl.Result{}, nil
	}

	buildWorkloads, err := c.getBuildWorkloadOwners(ctx, kpackImage)
	if err != nil {
		log.Error(err, "failed to find BuildWorkload owners for Kpack image")
		return ctrl.Result{}, err
	}

	if len(buildWorkloads) == 0 {
		log.Info("ignoring the Kpack build change as it is not owned by BuildWorkloads")
		return ctrl.Result{}, nil
	}

	cfBuilds, err := c.getCFBuilds(ctx, buildWorkloads)
	if err != nil {
		log.Error(err, "failed to get CFBuilds for BuildWorkloads")
		return ctrl.Result{}, err
	}
	if len(cfBuilds) == 0 {
		log.Info("ignoring the Kpack build change as it is not referenced by CFBuilds")
		return ctrl.Result{}, nil
	}

	for _, cfBuild := range cfBuilds {
		if cfBuild.Status.Droplet == nil {
			continue
		}

		if cfBuild.Status.Droplet.Registry.Image == kpackImage.Status.LatestImage {
			return ctrl.Result{}, nil
		}
	}

	latestCfBuild := getLatestCFBuild(cfBuilds)
	succeededCondition := meta.FindStatusCondition(latestCfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)
	if succeededCondition == nil || succeededCondition.Status == metav1.ConditionUnknown {
		return ctrl.Result{Requeue: true}, nil
	}

	newCfBuild := &korifiv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       latestCfBuild.Namespace,
			Name:            uuid.NewString(),
			OwnerReferences: latestCfBuild.OwnerReferences,
		},
		Spec: latestCfBuild.Spec,
	}

	err = c.k8sClient.Create(ctx, newCfBuild)
	if err != nil {
		log.Error(err, "failed to create new CFBuild for unattended Kpack build")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func getLatestCFBuild(cfBuilds []*korifiv1alpha1.CFBuild) *korifiv1alpha1.CFBuild {
	latestBuildIdx := 0
	for i := range cfBuilds {
		if cfBuilds[latestBuildIdx].CreationTimestamp.Before(&cfBuilds[i].CreationTimestamp) {
			latestBuildIdx = i
		}
	}

	return cfBuilds[latestBuildIdx]
}

func isUnattendedBuild(kpackImage *kpackv1alpha2.Image) bool {
	unattendedBuildReasons := map[string]any{
		"STACK":     nil,
		"BUILDPACK": nil,
	}

	buildReasons := strings.Split(kpackImage.Status.LatestBuildReason, ",")
	for _, reason := range buildReasons {
		if _, ok := unattendedBuildReasons[reason]; !ok {
			return false
		}
	}

	return true
}

func (c KpackImageController) getCFBuilds(ctx context.Context, buildWorkloads []korifiv1alpha1.BuildWorkload) ([]*korifiv1alpha1.CFBuild, error) {
	result := []*korifiv1alpha1.CFBuild{}
	for _, workload := range buildWorkloads {
		cfBuild, err := c.getCFBuildForWorkload(ctx, workload)
		if err != nil {
			return nil, err
		}
		result = append(result, cfBuild)
	}

	return result, nil
}

func (c KpackImageController) getBuildWorkloadOwners(ctx context.Context, kpackImage *kpackv1alpha2.Image) ([]korifiv1alpha1.BuildWorkload, error) {
	buildWorkloads := []korifiv1alpha1.BuildWorkload{}
	for _, owner := range kpackImage.GetOwnerReferences() {
		if owner.Kind != "BuildWorkload" {
			continue
		}

		workload := korifiv1alpha1.BuildWorkload{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: kpackImage.Namespace,
				Name:      owner.Name,
			},
		}

		err := c.k8sClient.Get(ctx, client.ObjectKeyFromObject(&workload), &workload)
		if err != nil {
			return nil, err
		}

		buildWorkloads = append(buildWorkloads, workload)
	}

	return buildWorkloads, nil
}

func (c KpackImageController) getCFBuildForWorkload(ctx context.Context, workload korifiv1alpha1.BuildWorkload) (*korifiv1alpha1.CFBuild, error) {
	for _, owner := range workload.GetOwnerReferences() {
		if owner.Kind != "CFBuild" {
			continue
		}

		cfBuild := &korifiv1alpha1.CFBuild{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: workload.Namespace,
				Name:      owner.Name,
			},
		}
		err := c.k8sClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)
		if err != nil {
			return nil, err
		}

		return cfBuild, nil
	}

	return nil, errors.New("no CFBuild owner found")
}

func (c *KpackImageController) SetupWithManager(mgr ctrl.Manager) error {
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
		For(&kpackv1alpha2.Image{}).
		WithEventFilter(labelSelector).
		Complete(c)
}
