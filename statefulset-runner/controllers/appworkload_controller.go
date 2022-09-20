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

package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Environment Variable Names
	EnvPodName              = "POD_NAME"
	EnvCFInstanceIP         = "CF_INSTANCE_IP"
	EnvCFInstanceGUID       = "CF_INSTANCE_GUID"
	EnvCFInstanceInternalIP = "CF_INSTANCE_INTERNAL_IP"
	EnvCFInstanceIndex      = "CF_INSTANCE_INDEX"

	// StatefulSet Keys
	AnnotationVersion     = "korifi.cloudfoundry.org/version"
	AnnotationAppID       = "korifi.cloudfoundry.org/application-id"
	AnnotationProcessGUID = "korifi.cloudfoundry.org/process-guid"

	LabelGUID                   = "korifi.cloudfoundry.org/guid"
	LabelVersion                = "korifi.cloudfoundry.org/version"
	LabelAppGUID                = "korifi.cloudfoundry.org/app-guid"
	LabelAppWorkloadGUID        = "korifi.cloudfoundry.org/appworkload-guid"
	LabelProcessType            = "korifi.cloudfoundry.org/process-type"
	LabelStatefulSetRunnerIndex = "korifi.cloudfoundry.org/add-stsr-index"

	ApplicationContainerName  = "application"
	AppWorkloadReconcilerName = "statefulset-runner"
	ServiceAccountName        = "korifi-app"

	LivenessFailureThreshold  = 4
	ReadinessFailureThreshold = 1

	PodAffinityTermWeight = 100
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o ../fake -fake-name PDB . PDB
type PDB interface {
	Update(ctx context.Context, statefulSet *appsv1.StatefulSet) error
}

// AppWorkloadReconciler reconciles a AppWorkload object
type AppWorkloadReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	pdb    PDB
	Log    logr.Logger
}

func NewAppWorkloadReconciler(c client.Client, scheme *runtime.Scheme, pdb PDB, log logr.Logger) *AppWorkloadReconciler {
	return &AppWorkloadReconciler{
		Client: c,
		Scheme: scheme,
		pdb:    pdb,
		Log:    log,
	}
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=appworkloads,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=appworkloads/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=appworkloads/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=create;patch;get;list;watch
//+kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=create;update;deletecollection

func (r *AppWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var appWorkload korifiv1alpha1.AppWorkload
	err := r.Client.Get(ctx, req.NamespacedName, &appWorkload)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			r.Log.Error(err, "Error when fetching AppWorkload", "AppWorkload.Name", req.Name, "AppWorkload.Namespace", req.Namespace)
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if appWorkload.Spec.RunnerName != AppWorkloadReconcilerName {
		return ctrl.Result{}, nil
	}

	statefulSet, err := r.Convert(appWorkload)
	// Not clear what errors this would produce, but we may use it later
	if err != nil {
		r.Log.Error(err, "Error when converting AppWorkload")
		return ctrl.Result{}, err
	}

	orig := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSet.Name,
			Namespace: statefulSet.Namespace,
		},
	}
	_, err = controllerutil.CreateOrPatch(ctx, r.Client, orig, func() error {
		orig.ObjectMeta.Labels = statefulSet.ObjectMeta.Labels
		orig.ObjectMeta.Annotations = statefulSet.ObjectMeta.Annotations
		orig.ObjectMeta.OwnerReferences = statefulSet.ObjectMeta.OwnerReferences
		orig.Spec = statefulSet.Spec

		return nil
	})
	if err != nil {
		r.Log.Error(err, "Error when creating or updating StatefulSet")
		return ctrl.Result{}, err
	}

	updatedStatefulSet := &appsv1.StatefulSet{}
	err = r.Client.Get(ctx, client.ObjectKeyFromObject(statefulSet), updatedStatefulSet)
	if err != nil {
		r.Log.Info("Error when fetching StatefulSet", "StatefulSet.Name", statefulSet.Name, "StatefulSet.Namespace", statefulSet.Namespace, "error", err.Error())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	err = r.pdb.Update(ctx, updatedStatefulSet)
	if err != nil {
		r.Log.Error(err, "Error when creating or updating pod disruption budget")
		return ctrl.Result{}, err
	}

	appWorkload.Status.ReadyReplicas = updatedStatefulSet.Status.ReadyReplicas
	err = r.Client.Status().Update(ctx, &appWorkload)
	if err != nil {
		r.Log.Error(err, "Error when updating AppWorkload status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.AppWorkload{}).
		Watches(
			&source.Kind{Type: new(appsv1.StatefulSet)},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				var requests []reconcile.Request
				if appWorkloadName, ok := obj.GetLabels()[LabelAppWorkloadGUID]; ok {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      appWorkloadName,
							Namespace: obj.GetNamespace(),
						},
					})
				}
				return requests
			})).
		Complete(r)
}

func sanitizeName(name, fallback string) string {
	const sanitizedNameMaxLen = 40
	return sanitizeNameWithMaxStringLen(name, fallback, sanitizedNameMaxLen)
}

func sanitizeNameWithMaxStringLen(name, fallback string, maxStringLen int) string {
	validNameRegex := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	sanitizedName := strings.ReplaceAll(strings.ToLower(name), "_", "-")

	if validNameRegex.MatchString(sanitizedName) {
		return truncateString(sanitizedName, maxStringLen)
	}

	return truncateString(fallback, maxStringLen)
}

func truncateString(str string, num int) string {
	if len(str) > num {
		return str[0:num]
	}

	return str
}

func GetStatefulSetName(appWorkload korifiv1alpha1.AppWorkload) (string, error) {
	nameSuffix, err := hash(fmt.Sprintf("%s-%s", appWorkload.Spec.GUID, appWorkload.Spec.Version))
	if err != nil {
		return "", fmt.Errorf("failed to generate hash for statefulset name: %w", err)
	}

	namePrefix := fmt.Sprintf("%s-%s", appWorkload.Spec.AppGUID, appWorkload.Namespace)
	namePrefix = sanitizeName(namePrefix, appWorkload.Spec.GUID)

	return fmt.Sprintf("%s-%s", namePrefix, nameSuffix), nil
}

func (r *AppWorkloadReconciler) Convert(appWorkload korifiv1alpha1.AppWorkload) (*appsv1.StatefulSet, error) {
	envs := appWorkload.Spec.Env
	fieldEnvs := []corev1.EnvVar{
		{
			Name: EnvPodName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name: EnvCFInstanceGUID,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.uid",
				},
			},
		},
		{
			Name: EnvCFInstanceIP,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.hostIP",
				},
			},
		},
		{
			Name: EnvCFInstanceInternalIP,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
	}

	envs = append(envs, fieldEnvs...)

	ports := []corev1.ContainerPort{}

	for _, port := range appWorkload.Spec.Ports {
		ports = append(ports, corev1.ContainerPort{ContainerPort: port})
	}

	livenessProbe := appWorkload.Spec.LivenessProbe
	readinessProbe := appWorkload.Spec.ReadinessProbe

	containers := []corev1.Container{
		{
			Name:            ApplicationContainerName,
			Image:           appWorkload.Spec.Image,
			ImagePullPolicy: corev1.PullAlways,
			Command:         appWorkload.Spec.Command,
			Env:             envs,
			Ports:           ports,
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: tools.PtrTo(false),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Resources:      appWorkload.Spec.Resources,
			LivenessProbe:  livenessProbe,
			ReadinessProbe: readinessProbe,
		},
	}

	statefulsetName, err := GetStatefulSetName(appWorkload)
	if err != nil {
		return nil, err
	}

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulsetName,
			Namespace: appWorkload.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			PodManagementPolicy: "Parallel",
			Replicas:            &appWorkload.Spec.Instances,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers:       containers,
					ImagePullSecrets: appWorkload.Spec.ImagePullSecrets,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: tools.PtrTo(true),
					},
					ServiceAccountName: ServiceAccountName,
				},
			},
		},
	}

	statefulSet.Spec.Template.Spec.AutomountServiceAccountToken = tools.PtrTo(false)
	statefulSet.Spec.Selector = StatefulSetLabelSelector(&appWorkload)

	statefulSet.Spec.Template.Spec.Affinity = &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: PodAffinityTermWeight,
					PodAffinityTerm: corev1.PodAffinityTerm{
						TopologyKey: corev1.LabelHostname,
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: toLabelSelectorRequirements(statefulSet.Spec.Selector),
						},
					},
				},
			},
		},
	}

	err = controllerutil.SetOwnerReference(&appWorkload, statefulSet, r.Scheme)
	if err != nil {
		r.Log.Error(err, "failed to set OwnerRef on StatefulSet")
		return nil, err
	}

	labels := map[string]string{
		LabelGUID:                   appWorkload.Spec.GUID,
		LabelProcessType:            appWorkload.Spec.ProcessType,
		LabelVersion:                appWorkload.Spec.Version,
		LabelAppGUID:                appWorkload.Spec.AppGUID,
		LabelAppWorkloadGUID:        appWorkload.Name,
		LabelStatefulSetRunnerIndex: "true",
	}

	statefulSet.Spec.Template.Labels = labels
	statefulSet.Labels = labels

	annotations := map[string]string{
		AnnotationAppID:       appWorkload.Spec.AppGUID,
		AnnotationVersion:     appWorkload.Spec.Version,
		AnnotationProcessGUID: fmt.Sprintf("%s-%s", appWorkload.Spec.GUID, appWorkload.Spec.Version),
	}

	statefulSet.Annotations = annotations
	statefulSet.Spec.Template.Annotations = annotations

	return statefulSet, nil
}

func toLabelSelectorRequirements(selector *metav1.LabelSelector) []metav1.LabelSelectorRequirement {
	labels := selector.MatchLabels
	reqs := make([]metav1.LabelSelectorRequirement, 0, len(labels))

	for label, value := range labels {
		reqs = append(reqs, metav1.LabelSelectorRequirement{
			Key:      label,
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{value},
		})
	}

	return reqs
}

func StatefulSetLabelSelector(appWorkload *korifiv1alpha1.AppWorkload) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			LabelGUID:    appWorkload.Spec.GUID,
			LabelVersion: appWorkload.Spec.Version,
		},
	}
}

func hash(s string) (string, error) {
	const MaxHashLength = 10

	sha := sha256.New()

	if _, err := sha.Write([]byte(s)); err != nil {
		return "", fmt.Errorf("failed to calculate sha: %w", err)
	}

	hashValue := hex.EncodeToString(sha.Sum(nil))

	return hashValue[:MaxHashLength], nil
}
