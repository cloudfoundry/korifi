/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/tools/image"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	corev1alpha1 "github.com/pivotal/kpack/pkg/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	clusterBuilderKind          = "ClusterBuilder"
	clusterBuilderAPIVersion    = "kpack.io/v1alpha2"
	BuildWorkloadLabelKey       = "korifi.cloudfoundry.org/build-workload-name"
	ImageGenerationKey          = "korifi.cloudfoundry.org/kpack-image-generation"
	kpackReconcilerName         = "kpack-image-builder"
	buildpackBuildMetadataLabel = "io.buildpacks.build.metadata"
)

//counterfeiter:generate -o fake -fake-name ImageConfigGetter . ImageConfigGetter

type ImageConfigGetter interface {
	Config(ctx context.Context, creds image.Creds, imageRef string) (image.Config, error)
}

//counterfeiter:generate -o fake -fake-name RepositoryCreator . RepositoryCreator

type RepositoryCreator interface {
	CreateRepository(ctx context.Context, name string) error
}

func NewBuildWorkloadReconciler(
	c client.Client,
	scheme *runtime.Scheme,
	log logr.Logger,
	config *config.ControllerConfig,
	imageConfigGetter ImageConfigGetter,
	imageRepoPrefix string,
	imageRepoCreator RepositoryCreator,
	builderReadinessTimeout time.Duration,
) *k8s.PatchingReconciler[korifiv1alpha1.BuildWorkload, *korifiv1alpha1.BuildWorkload] {
	buildWorkloadReconciler := BuildWorkloadReconciler{
		k8sClient:               c,
		scheme:                  scheme,
		log:                     log,
		controllerConfig:        config,
		imageConfigGetter:       imageConfigGetter,
		imageRepoPrefix:         imageRepoPrefix,
		imageRepoCreator:        imageRepoCreator,
		builderReadinessTimeout: builderReadinessTimeout,
	}
	return k8s.NewPatchingReconciler[korifiv1alpha1.BuildWorkload, *korifiv1alpha1.BuildWorkload](log, c, &buildWorkloadReconciler)
}

// BuildWorkloadReconciler reconciles a BuildWorkload object
type BuildWorkloadReconciler struct {
	k8sClient               client.Client
	scheme                  *runtime.Scheme
	log                     logr.Logger
	controllerConfig        *config.ControllerConfig
	imageConfigGetter       ImageConfigGetter
	imageRepoPrefix         string
	imageRepoCreator        RepositoryCreator
	builderReadinessTimeout time.Duration
}

func (r *BuildWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.BuildWorkload{}).
		Watches(
			new(buildv1alpha2.Image),
			handler.EnqueueRequestForOwner(r.scheme, mgr.GetRESTMapper(), &korifiv1alpha1.BuildWorkload{}),
		).
		Watches(
			new(buildv1alpha2.Build),
			handler.EnqueueRequestsFromMapFunc(r.buildWorkloadsFromBuild),
		).
		WithEventFilter(predicate.NewPredicateFuncs(filterBuildWorkloads))
}

func (r *BuildWorkloadReconciler) buildWorkloadsFromBuild(ctx context.Context, o client.Object) []reconcile.Request {
	buildworkloads := new(korifiv1alpha1.BuildWorkloadList)
	err := r.k8sClient.List(ctx, buildworkloads, client.InNamespace(o.GetNamespace()),
		client.MatchingLabels{
			korifiv1alpha1.CFAppGUIDLabelKey: o.GetLabels()[buildv1alpha2.ImageLabel],
			ImageGenerationKey:               o.GetLabels()[buildv1alpha2.ImageGenerationLabel],
		},
	)
	if err != nil {
		r.log.Info("failed to list BuildWorkloads",
			korifiv1alpha1.CFAppGUIDLabelKey, o.GetLabels()[buildv1alpha2.ImageLabel],
			ImageGenerationKey, o.GetLabels()[buildv1alpha2.ImageGenerationLabel],
			"reason", err,
		)
	}

	res := []reconcile.Request{}
	for i := range buildworkloads.Items {
		res = append(res, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&buildworkloads.Items[i])})
	}

	return res
}

func filterBuildWorkloads(object client.Object) bool {
	buildWorkload, ok := object.(*korifiv1alpha1.BuildWorkload)
	if !ok {
		return true
	}

	// Only reconcile buildworkloads that have their Spec.BuilderName matching this builder
	return buildWorkload.Spec.BuilderName == kpackReconcilerName
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads/status,verbs=get;patch

//+kubebuilder:rbac:groups=kpack.io,resources=images,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups=kpack.io,resources=images/status,verbs=get;patch
//+kubebuilder:rbac:groups=kpack.io,resources=builds,verbs=deletecollection
//+kubebuilder:rbac:groups=kpack.io,resources=builders,verbs=get;list;watch;create;patch;update

//+kubebuilder:rbac:groups="",resources=serviceaccounts;secrets,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups="",resources=serviceaccounts/status;secrets/status,verbs=get

func (r *BuildWorkloadReconciler) ReconcileResource(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload) (ctrl.Result, error) {
	log := r.log.WithValues("namespace", buildWorkload.Namespace, "name", buildWorkload.Name)

	buildWorkload.Status.ObservedGeneration = buildWorkload.Generation
	log.V(1).Info("set observed generation", "generation", buildWorkload.Status.ObservedGeneration)

	if !buildWorkload.GetDeletionTimestamp().IsZero() {
		return r.finalize(ctx, log, buildWorkload)
	}

	var err error
	if !hasKpackImage(buildWorkload) {
		var builderName string
		if len(buildWorkload.Spec.Buildpacks) > 0 {
			builderName, err = r.ensureKpackBuilder(ctx, log, buildWorkload)
			if err != nil {
				log.Info("failed ensuring custom builder", "reason", err)
				return ctrl.Result{}, ignoreDoNotRetryError(fmt.Errorf("failed ensuring custom builder: %w", err))
			}
		}

		err = r.ensureKpackImageRequirements(ctx, buildWorkload)
		if err != nil {
			log.Info("kpack image requirements for buildWorkload are not met", "guid", buildWorkload.Name, "reason", err)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, r.reconcileKpackImage(ctx, log, buildWorkload, builderName)
	}

	if hasCompleted(buildWorkload) {
		return ctrl.Result{}, nil
	}

	kpackImage := new(buildv1alpha2.Image)
	err = r.k8sClient.Get(ctx, client.ObjectKey{
		Namespace: buildWorkload.Namespace,
		Name:      buildWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey],
	}, kpackImage)
	if err != nil {
		log.Info("error when fetching Kpack Image", "reason", err)
		return ctrl.Result{}, fmt.Errorf("failed getting kpack Image: %w", err)
	}

	err = r.recoverIfBuildCreationHasBeenSkipped(ctx, log, buildWorkload, kpackImage)
	if err != nil {
		log.Info("ensuring kpack build was generated failed", "reason", err)
		return ctrl.Result{}, ignoreDoNotRetryError(err)
	}

	builderReadyCondition, err := r.getBuilderReadyCondition(ctx, log, buildWorkload, kpackImage)
	if err != nil {
		return ctrl.Result{}, ignoreDoNotRetryError(fmt.Errorf("failed getting builder readiness condition"))
	}

	if builderReadyCondition.IsFalse() {
		if time.Since(builderReadyCondition.LastTransitionTime.Inner.Time) < r.builderReadinessTimeout {
			return ctrl.Result{}, errors.New("waiting for builder to be ready")
		}

		log.Info("failing build as builder not ready")
		meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.SucceededConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             "BuilderNotReady",
			Message:            "Check ClusterBuilder",
			ObservedGeneration: buildWorkload.Generation,
		})
		return ctrl.Result{}, nil
	}

	kpackBuilds, err := r.listKpackBuilds(ctx, buildWorkload)
	if err != nil {
		log.Info("error when listing kpack builds for build workload", "reason", err)
		return ctrl.Result{}, err

	}
	latestBuild, err := latestBuild(kpackBuilds)
	if err != nil {
		log.Info("error when getting latest kpack build", "reason", err)
		return ctrl.Result{}, err
	}

	if latestBuild == nil {
		return ctrl.Result{}, nil
	}

	err = r.failSkippedEarlierWorkloads(ctx, buildWorkload)
	if err != nil {
		log.Info("error when failing skipped earlier workloads", "reason", err)
	}

	latestBuildSuccessful := latestBuild.Status.GetCondition(corev1alpha1.ConditionSucceeded)
	if latestBuildSuccessful.IsFalse() {
		meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.SucceededConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             "BuildFailed",
			Message:            "Check build log output",
			ObservedGeneration: buildWorkload.Generation,
		})
	} else if latestBuildSuccessful.IsTrue() {
		meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.SucceededConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             "BuildSucceeded",
			Message:            "Image built successfully",
			ObservedGeneration: buildWorkload.Generation,
		})

		foundServiceAccount := corev1.ServiceAccount{}
		err = r.k8sClient.Get(ctx, types.NamespacedName{
			Namespace: buildWorkload.Namespace,
			Name:      r.controllerConfig.BuilderServiceAccount,
		}, &foundServiceAccount)
		if err != nil {
			log.Info("error when fetching kpack ServiceAccount", "reason", err)
			return ctrl.Result{}, err
		}

		buildWorkload.Status.Droplet, err = r.generateDropletStatus(ctx, latestBuild, foundServiceAccount.ImagePullSecrets)
		if err != nil {
			log.Info("error when compiling the DropletStatus", "reason", err)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *BuildWorkloadReconciler) recoverIfBuildCreationHasBeenSkipped(ctx context.Context, log logr.Logger, buildWorkload *korifiv1alpha1.BuildWorkload, kpackImage *buildv1alpha2.Image) error {
	workloadImageGeneration, err := strconv.ParseInt(buildWorkload.Labels[ImageGenerationKey], 10, 64)
	if err != nil {
		log.Info("couldn't parse image generation on buildworkload label", "reason", err)
		return fmt.Errorf("couldn't parse image generation on buildworkload label: %w", err)
	}

	imageReady := kpackImage.Status.GetCondition(corev1alpha1.ConditionReady)
	if imageReady != nil &&
		imageReady.Status != corev1.ConditionUnknown &&
		kpackImage.Status.ObservedGeneration >= workloadImageGeneration &&
		kpackImage.Status.LatestBuildImageGeneration < workloadImageGeneration {
		latestKpackBuild := &buildv1alpha2.Build{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: kpackImage.Namespace,
				Name:      kpackImage.Status.LatestBuildRef,
			},
		}
		err = r.k8sClient.Get(ctx, client.ObjectKeyFromObject(latestKpackBuild), latestKpackBuild)
		if err != nil {
			return fmt.Errorf("failed to get latest kpack build %q: %w", kpackImage.Status.LatestBuildRef, err)
		}
		err = k8s.Patch(ctx, r.k8sClient, latestKpackBuild, func() {
			if latestKpackBuild.Annotations == nil {
				latestKpackBuild.Annotations = map[string]string{}
			}
			latestKpackBuild.Annotations[buildv1alpha2.BuildNeededAnnotation] = "true"
		})
		if err != nil {
			return fmt.Errorf("failed to request additional build for build %q: %w", latestKpackBuild.Name, err)
		}
	}

	return nil
}

func (r *BuildWorkloadReconciler) getBuilderReadyCondition(ctx context.Context, log logr.Logger, buildWorkload *korifiv1alpha1.BuildWorkload, kpackImage *buildv1alpha2.Image) (*corev1alpha1.Condition, error) {
	var condition *corev1alpha1.Condition

	switch kpackImage.Spec.Builder.Kind {
	case "ClusterBuilder":
		var imageBuilder buildv1alpha2.ClusterBuilder
		err := r.k8sClient.Get(ctx, client.ObjectKey{Name: kpackImage.Spec.Builder.Name}, &imageBuilder)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				log.Info("failing build as cluster builder is not found", "reason", err)
				meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
					Type:               korifiv1alpha1.SucceededConditionType,
					Status:             metav1.ConditionFalse,
					Reason:             "BuilderNotReady",
					Message:            "ClusterBuilder not found",
					ObservedGeneration: buildWorkload.Generation,
				})
				return nil, newDoNotRetryError(err)
			}

			log.Info("error when fetching Kpack ClusterBuilder", "reason", err)
			return nil, err
		}
		condition = imageBuilder.Status.GetCondition(corev1alpha1.ConditionReady)

	case "Builder":
		var imageBuilder buildv1alpha2.Builder
		err := r.k8sClient.Get(ctx, client.ObjectKey{Name: kpackImage.Spec.Builder.Name, Namespace: buildWorkload.Namespace}, &imageBuilder)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				log.Info("failing build as builder is not found", "reason", err)
				meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
					Type:               korifiv1alpha1.SucceededConditionType,
					Status:             metav1.ConditionFalse,
					Reason:             "BuilderNotReady",
					Message:            "Custom Builder not found",
					ObservedGeneration: buildWorkload.Generation,
				})
				return nil, newDoNotRetryError(err)
			}

			log.Info("error when fetching Kpack Builder", "reason", err)
			return nil, err
		}

		condition = imageBuilder.Status.GetCondition(corev1alpha1.ConditionReady)
	default:
		return nil, fmt.Errorf("unknown builder type %q", kpackImage.Spec.Builder.Kind)
	}

	return condition, nil
}

func (r *BuildWorkloadReconciler) getDefaultClusterBuilder(ctx context.Context) (*buildv1alpha2.ClusterBuilder, error) {
	var defaultBuilder buildv1alpha2.ClusterBuilder
	err := r.k8sClient.Get(ctx, client.ObjectKey{Name: r.controllerConfig.ClusterBuilderName}, &defaultBuilder)
	return &defaultBuilder, err
}

type doNotRetryError struct {
	inner error
}

func newDoNotRetryError(inner error) doNotRetryError {
	return doNotRetryError{
		inner: inner,
	}
}

func (e doNotRetryError) Error() string {
	return e.inner.Error()
}

func (e doNotRetryError) Unwrap() error {
	return e.inner
}

func ignoreDoNotRetryError(err error) error {
	if errors.As(err, &doNotRetryError{}) {
		return nil
	}
	return err
}

func (r *BuildWorkloadReconciler) ensureKpackBuilder(ctx context.Context, log logr.Logger, buildWorkload *korifiv1alpha1.BuildWorkload) (string, error) {
	var (
		defaultBuilder *buildv1alpha2.ClusterBuilder
		err            error
	)

	if defaultBuilder, err = r.getDefaultClusterBuilder(ctx); err != nil {
		if k8serrors.IsNotFound(err) {
			meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
				Type:               korifiv1alpha1.SucceededConditionType,
				Status:             metav1.ConditionFalse,
				Reason:             "BuilderNotReady",
				Message:            "Default ClusterBuilder not found",
				ObservedGeneration: buildWorkload.Generation,
			})
			return "", newDoNotRetryError(fmt.Errorf("default ClusterBuilder not found: %w", err))
		}

		log.Info("error when fetching default ClusterBuilder", "reason", err)
		return "", err
	}

	if err = r.checkBuildpacks(ctx, buildWorkload, defaultBuilder); err != nil {
		meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
			Type:               korifiv1alpha1.SucceededConditionType,
			Status:             metav1.ConditionFalse,
			Reason:             "InvalidBuildpacks",
			Message:            err.Error(),
			ObservedGeneration: buildWorkload.Generation,
		})

		return "", newDoNotRetryError(err)
	}

	builderName := ComputeBuilderName(buildWorkload.Spec.Buildpacks)
	builderRepo := fmt.Sprintf("%sbuilders-%s", r.imageRepoPrefix, builderName)
	err = r.imageRepoCreator.CreateRepository(ctx, builderRepo)
	if err != nil {
		log.Info("failed creating builder repo", "reason", err)
		return "", fmt.Errorf("failed to create builder repo: %w", err)
	}

	builder := &buildv1alpha2.Builder{
		ObjectMeta: metav1.ObjectMeta{
			Name:      builderName,
			Namespace: buildWorkload.Namespace,
		},
	}
	_, err = ctrl.CreateOrUpdate(ctx, r.k8sClient, builder, func() error {
		if err = controllerutil.SetControllerReference(buildWorkload, builder, r.scheme); err != nil {
			log.Info("unable to set owner reference on Builder", "reason", err)
			return err
		}

		builder.Spec.Tag = builderRepo
		builder.Spec.Stack = defaultBuilder.Spec.Stack
		builder.Spec.Store = defaultBuilder.Spec.Store
		builder.Spec.ServiceAccountName = r.controllerConfig.BuilderServiceAccount
		builder.Spec.Order = nil
		for _, bp := range buildWorkload.Spec.Buildpacks {
			builder.Spec.Order = append(builder.Spec.Order, buildv1alpha2.BuilderOrderEntry{
				Group: []buildv1alpha2.BuilderBuildpackRef{{
					BuildpackRef: corev1alpha1.BuildpackRef{
						BuildpackInfo: corev1alpha1.BuildpackInfo{
							Id: bp,
						},
					},
				}},
			})
		}

		return nil
	})
	if err != nil {
		log.Info("failed creating or updating kpack Builder", "reason", err)
		return "", fmt.Errorf("failed creating or updating kpack Builder: %w", err)
	}

	return builder.Name, nil
}

func ComputeBuilderName(bps []string) string {
	return uuid.NewSHA1(uuid.Nil, []byte(strings.Join(bps, "\x00"))).String()
}

func (r *BuildWorkloadReconciler) checkBuildpacks(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload, defaultBuilder *buildv1alpha2.ClusterBuilder) error {
	validIDs := map[string]bool{}
	for _, bp := range clusterBuilderToBuildpacks(defaultBuilder, metav1.Now()) {
		validIDs[bp.Name] = true
	}

	for _, bp := range buildWorkload.Spec.Buildpacks {
		if !validIDs[bp] {
			return fmt.Errorf("buildpack %q not present in default ClusterStore. See `cf buildpacks`", bp)
		}
	}
	return nil
}

func (r *BuildWorkloadReconciler) failSkippedEarlierWorkloads(ctx context.Context, reconciledBuildWorkload *korifiv1alpha1.BuildWorkload) error {
	reconciledBuildWorkloadImageGeneration, err := strconv.ParseInt(reconciledBuildWorkload.Labels[ImageGenerationKey], 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse image generation label for build workload: %w", err)
	}

	buildWorkloads := &korifiv1alpha1.BuildWorkloadList{}
	err = r.k8sClient.List(ctx, buildWorkloads, client.InNamespace(reconciledBuildWorkload.Namespace), client.MatchingLabels{
		korifiv1alpha1.CFAppGUIDLabelKey: reconciledBuildWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey],
	})
	if err != nil {
		return fmt.Errorf("failed to list build workloads: %w", err)
	}

	for i := range buildWorkloads.Items {
		workload := &buildWorkloads.Items[i]

		workloadImageGeneration, err := strconv.ParseInt(workload.Labels[ImageGenerationKey], 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse image generation label for build workload: %w", err)
		}

		if workloadImageGeneration >= reconciledBuildWorkloadImageGeneration {
			continue
		}

		if !hasKpackImage(workload) || hasCompleted(workload) {
			continue
		}

		kpackBuilds, err := r.listKpackBuilds(ctx, workload)
		if err != nil {
			return fmt.Errorf("failed to find kpack build: %w", err)
		}

		if len(kpackBuilds) > 0 {
			continue
		}

		err = k8s.Patch(ctx, r.k8sClient, workload, func() {
			meta.SetStatusCondition(&workload.Status.Conditions, metav1.Condition{
				Type:               korifiv1alpha1.SucceededConditionType,
				Status:             metav1.ConditionFalse,
				Reason:             "KpackMissedBuild",
				Message:            "More recent build workload has been scheduled",
				ObservedGeneration: reconciledBuildWorkload.Generation,
			})
		})
		if err != nil {
			return fmt.Errorf("failed to patch older build workload: %w", err)
		}

	}

	return nil
}

func hasKpackImage(buildWorkload *korifiv1alpha1.BuildWorkload) bool {
	return meta.FindStatusCondition(buildWorkload.Status.Conditions, korifiv1alpha1.SucceededConditionType) != nil
}

func hasCompleted(buildWorkload *korifiv1alpha1.BuildWorkload) bool {
	return meta.FindStatusCondition(buildWorkload.Status.Conditions, korifiv1alpha1.SucceededConditionType).Status != metav1.ConditionUnknown
}

func (r *BuildWorkloadReconciler) listKpackBuilds(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload) ([]buildv1alpha2.Build, error) {
	var kpackBuildList buildv1alpha2.BuildList
	err := r.k8sClient.List(ctx, &kpackBuildList, client.InNamespace(buildWorkload.Namespace), client.MatchingLabels{
		buildv1alpha2.ImageLabel:           buildWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey],
		buildv1alpha2.ImageGenerationLabel: buildWorkload.Labels[ImageGenerationKey],
	})
	if err != nil {
		return nil, fmt.Errorf("error when fetching KPack builds: %w", err)
	}

	return kpackBuildList.Items, nil
}

func latestBuild(builds []buildv1alpha2.Build) (*buildv1alpha2.Build, error) {
	if len(builds) == 0 {
		return nil, nil
	}

	latestBuild := builds[0]
	for _, build := range builds {
		buildNumber, err := strconv.ParseInt(build.Labels[buildv1alpha2.BuildNumberLabel], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse build number for build %q: %w", build.Name, err)
		}
		latestBuildNumber, err := strconv.ParseInt(latestBuild.Labels[buildv1alpha2.BuildNumberLabel], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse build number for build %q: %w", latestBuild.Name, err)
		}
		if buildNumber > latestBuildNumber {
			latestBuild = build
		}
	}

	return &latestBuild, nil
}

func (r *BuildWorkloadReconciler) ensureKpackImageRequirements(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload) error {
	for _, secret := range buildWorkload.Spec.Source.Registry.ImagePullSecrets {
		err := r.k8sClient.Get(ctx, types.NamespacedName{Namespace: buildWorkload.Namespace, Name: secret.Name}, &corev1.Secret{})
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *BuildWorkloadReconciler) reconcileKpackImage(
	ctx context.Context,
	log logr.Logger,
	buildWorkload *korifiv1alpha1.BuildWorkload,
	customBuilderName string,
) error {
	appGUID := buildWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey]
	kpackImageNamespace := buildWorkload.Namespace
	kpackImageTag := r.repositoryRef(appGUID)
	desiredKpackImage := buildv1alpha2.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appGUID,
			Namespace: kpackImageNamespace,
		},
	}

	if err := r.imageRepoCreator.CreateRepository(ctx, r.repositoryRef(appGUID)); err != nil {
		log.Info("failed to create image repository", "reason", err)
		return err
	}

	_, err := controllerutil.CreateOrPatch(ctx, r.k8sClient, &desiredKpackImage, func() error {
		desiredKpackImage.Labels = map[string]string{
			BuildWorkloadLabelKey: buildWorkload.Name,
		}

		desiredKpackImage.Spec = buildv1alpha2.ImageSpec{
			Tag: kpackImageTag,
			Builder: corev1.ObjectReference{
				Kind:       clusterBuilderKind,
				Name:       r.controllerConfig.ClusterBuilderName,
				APIVersion: clusterBuilderAPIVersion,
			},
			ServiceAccountName: r.controllerConfig.BuilderServiceAccount,
			Source: corev1alpha1.SourceConfig{
				Registry: &corev1alpha1.Registry{
					Image:            buildWorkload.Spec.Source.Registry.Image,
					ImagePullSecrets: buildWorkload.Spec.Source.Registry.ImagePullSecrets,
				},
			},
			Build: &buildv1alpha2.ImageBuild{
				Services: buildWorkload.Spec.Services,
				Env:      buildWorkload.Spec.Env,
			},
		}
		if customBuilderName != "" {
			desiredKpackImage.Spec.Builder.Kind = "Builder"
			desiredKpackImage.Spec.Builder.Name = customBuilderName
			desiredKpackImage.Spec.Builder.Namespace = buildWorkload.Namespace
		}

		// Cannot use SetControllerReference here as multiple BuildWorkloads can "own" the same Image.
		err := controllerutil.SetOwnerReference(buildWorkload, &desiredKpackImage, r.scheme)
		if err != nil {
			log.Info("failed to set OwnerRef on Kpack Image", "reason", err)
			return err
		}

		return nil
	})
	if err != nil {
		log.Info("failed to set create or patch kpack.Image", "reason", err)
		return err
	}

	meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
		Type:               korifiv1alpha1.SucceededConditionType,
		Status:             metav1.ConditionUnknown,
		Reason:             "BuildRunning",
		Message:            "Waiting for image build to complete",
		ObservedGeneration: buildWorkload.Generation,
	})

	buildWorkload.Labels[ImageGenerationKey] = strconv.FormatInt(desiredKpackImage.Generation, 10)

	return nil
}

func (r *BuildWorkloadReconciler) generateDropletStatus(ctx context.Context, kpackBuild *buildv1alpha2.Build, imagePullSecrets []corev1.LocalObjectReference) (*korifiv1alpha1.BuildDropletStatus, error) {
	imageRef := kpackBuild.Status.LatestImage

	creds := image.Creds{
		Namespace:          kpackBuild.Namespace,
		ServiceAccountName: r.controllerConfig.BuilderServiceAccount,
	}

	config, err := r.imageConfigGetter.Config(ctx, creds, imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed getting image config: %w", err)
	}

	var buildMd buildMetadata
	err = json.Unmarshal([]byte(config.Labels[buildpackBuildMetadataLabel]), &buildMd)
	if err != nil {
		return nil, fmt.Errorf("failed to umarshal build metadata: %w", err)
	}

	processTypes := []korifiv1alpha1.ProcessType{}
	for _, process := range buildMd.Processes {
		processTypes = append(processTypes, korifiv1alpha1.ProcessType{
			Type:    process.Type,
			Command: extractFullCommand(process),
		})
	}

	return &korifiv1alpha1.BuildDropletStatus{
		Registry: korifiv1alpha1.Registry{
			Image:            imageRef,
			ImagePullSecrets: imagePullSecrets,
		},

		Stack: kpackBuild.Status.Stack.ID,

		ProcessTypes: processTypes,
		Ports:        config.ExposedPorts,
	}, nil
}

type buildMetadata struct {
	Processes []process `json:"processes"`
}

type process struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func extractFullCommand(process process) string {
	cmdString := process.Command
	for _, a := range process.Args {
		cmdString = fmt.Sprintf(`%s %q`, cmdString, a)
	}
	return cmdString
}

func (r *BuildWorkloadReconciler) finalize(ctx context.Context, log logr.Logger, buildWorkload *korifiv1alpha1.BuildWorkload) (ctrl.Result, error) {
	if !controllerutil.ContainsFinalizer(buildWorkload, korifiv1alpha1.BuildWorkloadFinalizerName) {
		return ctrl.Result{}, nil
	}

	lastBuildWorkload, err := r.isLastBuildWorkload(ctx, buildWorkload)
	if err != nil {
		log.Error(err, "failed to check for last build workloads")
		return ctrl.Result{}, err
	}

	if lastBuildWorkload {
		err = r.deleteBuildsForBuildWorkload(ctx, buildWorkload)
		if err != nil {
			log.Error(err, "failed to delete builds for build workload")
			return ctrl.Result{}, err
		}

		hasRemainingBuilds, err := r.hasRemainingBuilds(ctx, buildWorkload)
		if err != nil {
			log.Error(err, "failed to check for remaining builds for build workload")
			return ctrl.Result{}, err
		}

		if hasRemainingBuilds {
			return ctrl.Result{RequeueAfter: time.Second}, nil
		}
	}

	if controllerutil.RemoveFinalizer(buildWorkload, korifiv1alpha1.BuildWorkloadFinalizerName) {
		log.V(1).Info("finalizer removed")
	}

	return ctrl.Result{}, nil
}

func (r *BuildWorkloadReconciler) isLastBuildWorkload(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload) (bool, error) {
	appBuildWorkloads := &korifiv1alpha1.BuildWorkloadList{}
	err := r.k8sClient.List(ctx, appBuildWorkloads, client.InNamespace(buildWorkload.Namespace), client.MatchingLabels{
		korifiv1alpha1.CFAppGUIDLabelKey: buildWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey],
		ImageGenerationKey:               buildWorkload.Labels[ImageGenerationKey],
	})
	if err != nil {
		return false, fmt.Errorf("failed to list build workloads: %w", err)
	}

	return len(appBuildWorkloads.Items) == 1, nil
}

func (r *BuildWorkloadReconciler) deleteBuildsForBuildWorkload(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload) error {
	err := r.k8sClient.DeleteAllOf(ctx, new(buildv1alpha2.Build), client.InNamespace(buildWorkload.Namespace), client.MatchingLabels{
		buildv1alpha2.ImageLabel:           buildWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey],
		buildv1alpha2.ImageGenerationLabel: buildWorkload.Labels[ImageGenerationKey],
	})
	if err != nil {
		return fmt.Errorf("failed to delete builds: %w", err)
	}
	return nil
}

func (r *BuildWorkloadReconciler) hasRemainingBuilds(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload) (bool, error) {
	buildList := &buildv1alpha2.BuildList{}
	if err := r.k8sClient.List(ctx, buildList, client.InNamespace(buildWorkload.Namespace), client.MatchingLabels{
		buildv1alpha2.ImageLabel:           buildWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey],
		buildv1alpha2.ImageGenerationLabel: buildWorkload.Labels[ImageGenerationKey],
	}); err != nil {
		return false, fmt.Errorf("failed to list build workloads: %w", err)
	}

	return len(buildList.Items) != 0, nil
}

func (r *BuildWorkloadReconciler) repositoryRef(appGUID string) string {
	return r.imageRepoPrefix + appGUID + "-droplets"
}
