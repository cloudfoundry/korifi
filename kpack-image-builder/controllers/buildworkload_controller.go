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
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/tools/image"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
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
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	clusterBuilderKind          = "ClusterBuilder"
	clusterBuilderAPIVersion    = "kpack.io/v1alpha2"
	BuildWorkloadLabelKey       = "korifi.cloudfoundry.org/build-workload-name"
	ImageGenerationKey          = "korifi.cloudfoundry.org/kpack-image-generation"
	kpackReconcilerName         = "kpack-image-builder"
	buildpackBuildMetadataLabel = "io.buildpacks.build.metadata"
	buildWorkloadFinalizerName  = "kpack-image-builder.korifi.cloudfoundry.org/buildworkload"
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

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=buildworkloads/status,verbs=get;patch

//+kubebuilder:rbac:groups=kpack.io,resources=images,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups=kpack.io,resources=images/status,verbs=get;patch
//+kubebuilder:rbac:groups=kpack.io,resources=builds,verbs=deletecollection

//+kubebuilder:rbac:groups="",resources=serviceaccounts;secrets,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups="",resources=serviceaccounts/status;secrets/status,verbs=get

func (r *BuildWorkloadReconciler) ReconcileResource(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload) (ctrl.Result, error) {
	if !buildWorkload.GetDeletionTimestamp().IsZero() {
		err := r.finalize(ctx, buildWorkload)
		return ctrl.Result{}, err
	}

	err := k8s.AddFinalizer(ctx, r.log, r.k8sClient, buildWorkload, buildWorkloadFinalizerName)
	if err != nil {
		r.log.Info("error adding finalizer", "reason", err)
		return ctrl.Result{}, err
	}

	if len(buildWorkload.Spec.Buildpacks) > 0 {
		// Specifying buildpacks is not supported
		meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.SucceededConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "InvalidBuildpacks",
			Message: `Only buildpack auto-detection is supported. Specifying buildpacks is not allowed.`,
		})

		return ctrl.Result{}, nil
	}

	if !hasKpackImage(buildWorkload) {
		err = r.ensureKpackImageRequirements(ctx, buildWorkload)
		if err != nil {
			r.log.Info("kpack image requirements for buildWorkload are not met", "guid", buildWorkload.Name, "reason", err)
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, r.reconcileKpackImage(ctx, buildWorkload)
	}

	if hasCompleted(buildWorkload) {
		return ctrl.Result{}, nil
	}

	var kpackImage buildv1alpha2.Image
	appGUID := buildWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey]
	err = r.k8sClient.Get(ctx, client.ObjectKey{Namespace: buildWorkload.Namespace, Name: appGUID}, &kpackImage)
	if err != nil {
		r.log.Error(err, "error when fetching Kpack Image")
		return ctrl.Result{}, err
	}

	var imageBuilder buildv1alpha2.ClusterBuilder
	err = r.k8sClient.Get(ctx, client.ObjectKey{Name: kpackImage.Spec.Builder.Name}, &imageBuilder)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			r.log.Info("failing build as builder is not found")
			meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
				Type:    korifiv1alpha1.SucceededConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "BuilderNotReady",
				Message: "ClusterBuilder not found",
			})
			return ctrl.Result{}, nil
		}

		r.log.Error(err, "error when fetching Kpack Builder")
		return ctrl.Result{}, err
	}

	if builderReady := imageBuilder.Status.GetCondition(corev1alpha1.ConditionReady); builderReady.IsFalse() {
		if time.Since(builderReady.LastTransitionTime.Inner.Time) < r.builderReadinessTimeout {
			return ctrl.Result{}, errors.New("waiting for builder to be ready")
		}

		r.log.Info("failing build as builder not ready")
		meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.SucceededConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "BuilderNotReady",
			Message: "Check ClusterBuilder",
		})
		return ctrl.Result{}, nil
	}

	kpackBuilds, err := r.listKpackBuilds(ctx, buildWorkload)
	if err != nil {
		r.log.Error(err, "error when listing kpack builds for build workload")
		return ctrl.Result{}, err

	}
	latestBuild, err := latestBuild(kpackBuilds)
	if err != nil {
		r.log.Error(err, "error when getting latest kpack build")
		return ctrl.Result{}, err
	}

	if latestBuild == nil {
		return ctrl.Result{}, nil
	}

	err = r.failSkippedEarlierWorkloads(ctx, buildWorkload)
	if err != nil {
		r.log.Error(err, "error when failing skipped earlier workloads")
	}

	latestBuildSuccessful := latestBuild.Status.GetCondition(corev1alpha1.ConditionSucceeded)
	if latestBuildSuccessful.IsFalse() {
		meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.SucceededConditionType,
			Status:  metav1.ConditionFalse,
			Reason:  "BuildFailed",
			Message: "Check build log output",
		})
	} else if latestBuildSuccessful.IsTrue() {
		meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
			Type:    korifiv1alpha1.SucceededConditionType,
			Status:  metav1.ConditionTrue,
			Reason:  "BuildSucceeded",
			Message: "Image built successfully",
		})

		foundServiceAccount := corev1.ServiceAccount{}
		err = r.k8sClient.Get(ctx, types.NamespacedName{
			Namespace: buildWorkload.Namespace,
			Name:      r.controllerConfig.BuilderServiceAccount,
		}, &foundServiceAccount)
		if err != nil {
			r.log.Info("error when fetching kpack ServiceAccount", "reason", err)
			return ctrl.Result{}, err
		}

		buildWorkload.Status.Droplet, err = r.generateDropletStatus(ctx, latestBuild, foundServiceAccount.ImagePullSecrets)
		if err != nil {
			r.log.Info("error when compiling the DropletStatus", "reason", err)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
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
				Type:    korifiv1alpha1.SucceededConditionType,
				Status:  metav1.ConditionFalse,
				Reason:  "KpackMissedBuild",
				Message: "More recent build workload has been scheduled",
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

func (r *BuildWorkloadReconciler) reconcileKpackImage(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload) error {
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
		r.log.Info("failed to create image repository", "reason", err)
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

		err := controllerutil.SetOwnerReference(buildWorkload, &desiredKpackImage, r.scheme)
		if err != nil {
			r.log.Info("failed to set OwnerRef on Kpack Image", "reason", err)
			return err
		}

		return nil
	})
	if err != nil {
		r.log.Info("failed to set create or patch kpack.Image", "reason", err)
		return err
	}

	meta.SetStatusCondition(&buildWorkload.Status.Conditions, metav1.Condition{
		Type:    korifiv1alpha1.SucceededConditionType,
		Status:  metav1.ConditionUnknown,
		Reason:  "BuildRunning",
		Message: "Waiting for image build to complete",
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

func (r *BuildWorkloadReconciler) buildWorkloadsFromBuild(o client.Object) []reconcile.Request {
	buildworkloads := new(korifiv1alpha1.BuildWorkloadList)
	err := r.k8sClient.List(context.Background(), buildworkloads, client.InNamespace(o.GetNamespace()),
		client.MatchingLabels{
			korifiv1alpha1.CFAppGUIDLabelKey: o.GetLabels()[buildv1alpha2.ImageLabel],
			ImageGenerationKey:               o.GetLabels()[buildv1alpha2.ImageGenerationLabel],
		},
	)
	if err != nil {
		r.log.Error(err, "failed to list BuildWorkloads",
			korifiv1alpha1.CFAppGUIDLabelKey, o.GetLabels()[buildv1alpha2.ImageLabel],
			ImageGenerationKey, o.GetLabels()[buildv1alpha2.ImageGenerationLabel],
		)
	}

	res := []reconcile.Request{}
	for i := range buildworkloads.Items {
		res = append(res, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&buildworkloads.Items[i])})
	}

	return res
}

func (r *BuildWorkloadReconciler) finalize(ctx context.Context, buildWorkload *korifiv1alpha1.BuildWorkload) error {
	if !controllerutil.ContainsFinalizer(buildWorkload, buildWorkloadFinalizerName) {
		return nil
	}

	appBuildWorkloads := &korifiv1alpha1.BuildWorkloadList{}
	err := r.k8sClient.List(ctx, appBuildWorkloads, client.InNamespace(buildWorkload.Namespace), client.MatchingLabels{
		korifiv1alpha1.CFAppGUIDLabelKey: buildWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey],
		ImageGenerationKey:               buildWorkload.Labels[ImageGenerationKey],
	})
	if err != nil {
		r.log.Error(err, "failed to list build workloads")
		return err
	}

	if len(appBuildWorkloads.Items) == 1 {
		err = r.k8sClient.DeleteAllOf(ctx, new(buildv1alpha2.Build), client.InNamespace(buildWorkload.Namespace), client.MatchingLabels{
			buildv1alpha2.ImageLabel:           buildWorkload.Labels[korifiv1alpha1.CFAppGUIDLabelKey],
			buildv1alpha2.ImageGenerationLabel: buildWorkload.Labels[ImageGenerationKey],
		})
		if err != nil {
			r.log.Error(err, "failed to delete kpack.Build")
		}
	}

	if controllerutil.RemoveFinalizer(buildWorkload, buildWorkloadFinalizerName) {
		r.log.V(1).Info("finalizer removed")
	}

	return nil
}

func (r *BuildWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.BuildWorkload{}).
		Watches(
			&source.Kind{Type: new(buildv1alpha2.Image)},
			&handler.EnqueueRequestForOwner{OwnerType: &korifiv1alpha1.BuildWorkload{}},
		).
		Watches(
			&source.Kind{Type: new(buildv1alpha2.Build)},
			handler.EnqueueRequestsFromMapFunc(r.buildWorkloadsFromBuild),
		).
		WithEventFilter(predicate.NewPredicateFuncs(filterBuildWorkloads))
}

func (r *BuildWorkloadReconciler) repositoryRef(appGUID string) string {
	return r.imageRepoPrefix + appGUID + "-droplets"
}

func filterBuildWorkloads(object client.Object) bool {
	buildWorkload, ok := object.(*korifiv1alpha1.BuildWorkload)
	if !ok {
		return true
	}

	// Only reconcile buildworkloads that have their Spec.BuilderName matching this builder
	return buildWorkload.Spec.BuilderName == kpackReconcilerName
}
