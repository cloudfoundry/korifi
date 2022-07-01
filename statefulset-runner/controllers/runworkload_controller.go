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
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/api/resource"

	"code.cloudfoundry.org/eirini-controller/util"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
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

	// StatefulSet Keys
	AnnotationVersion     = "korifi.cloudfoundry.org/version"
	AnnotationAppID       = "korifi.cloudfoundry.org/application-id"
	AnnotationProcessGUID = "korifi.cloudfoundry.org/process-guid"

	LabelGUID            = "korifi.cloudfoundry.org/guid"
	LabelVersion         = "korifi.cloudfoundry.org/version"
	LabelAppGUID         = "korifi.cloudfoundry.org/app-guid"
	LabelRunWorkloadGUID = "korifi.cloudfoundry.org/run-workload-guid"
	LabelProcessType     = "korifi.cloudfoundry.org/process-type"

	ApplicationContainerName = "app"

	LivenessFailureThreshold  = 4
	ReadinessFailureThreshold = 1

	PodAffinityTermWeight = 100
)

// RunWorkloadReconciler reconciles a RunWorkload object
type RunWorkloadReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

func NewRunWorkloadReconciler(c client.Client, scheme *runtime.Scheme, log logr.Logger) *RunWorkloadReconciler {
	return &RunWorkloadReconciler{
		Client: c,
		Scheme: scheme,
		Log:    log,
	}
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=runworkloads,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=runworkloads/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=runworkloads/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=create;patch;get;list;watch

func (r *RunWorkloadReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var runWorkload korifiv1alpha1.RunWorkload
	err := r.Client.Get(ctx, req.NamespacedName, &runWorkload)
	if err != nil {
		r.Log.Error(err, "Error when fetching RunWorkload", "RunWorkload.Name", req.Name, "RunWorkload.Namespace", req.Namespace)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	statefulSet, err := r.Convert(runWorkload)
	// Not clear what errors this would produce, but we may use it later
	if err != nil {
		r.Log.Error(err, "Error when converting RunWorkload")
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
	err = r.Client.Get(ctx, types.NamespacedName{
		Name:      statefulSet.Name,
		Namespace: statefulSet.Namespace,
	}, updatedStatefulSet)
	if err != nil {
		r.Log.Info("Error when fetching StatefulSet", "StatefulSet.Name", statefulSet.Name, "StatefulSet.Namespace", statefulSet.Namespace, "error", err.Error())
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	runWorkload.Status.ReadyReplicas = updatedStatefulSet.Status.ReadyReplicas
	err = r.Client.Status().Update(ctx, &runWorkload)
	if err != nil {
		r.Log.Error(err, "Error when updating RunWorkload status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RunWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.RunWorkload{}).
		Watches(
			&source.Kind{Type: new(appsv1.StatefulSet)},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				var requests []reconcile.Request
				if runWorkloadName, ok := obj.GetLabels()[LabelRunWorkloadGUID]; ok {
					requests = append(requests, reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      runWorkloadName,
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

func GetStatefulSetName(runWorkLoad korifiv1alpha1.RunWorkload) string {
	nameSuffix, err := util.Hash(fmt.Sprintf("%s-%s", runWorkLoad.Spec.GUID, runWorkLoad.Spec.Version))
	if err != nil {
		panic(errors.Wrap(err, "failed to generate hash"))
	}

	namePrefix := fmt.Sprintf("%s-%s", runWorkLoad.Spec.AppGUID, runWorkLoad.Namespace)
	namePrefix = sanitizeName(namePrefix, runWorkLoad.Spec.GUID)

	return fmt.Sprintf("%s-%s", namePrefix, nameSuffix)
}

func (r *RunWorkloadReconciler) Convert(runWorkload korifiv1alpha1.RunWorkload) (*appsv1.StatefulSet, error) {
	envs := runWorkload.Spec.Env
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

	for _, port := range runWorkload.Spec.Ports {
		ports = append(ports, corev1.ContainerPort{ContainerPort: port})
	}

	livenessProbe := CreateLivenessProbe(runWorkload)
	readinessProbe := CreateReadinessProbe(runWorkload)

	allowPrivilegeEscalation := false
	runAsNonRoot := true

	containers := []corev1.Container{
		{
			Name:            ApplicationContainerName,
			Image:           runWorkload.Spec.Image,
			ImagePullPolicy: corev1.PullAlways,
			Command:         runWorkload.Spec.Command,
			Env:             envs,
			Ports:           ports,
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: &allowPrivilegeEscalation,
			},
			Resources:      getContainerResources(runWorkload.Spec.CPUWeight, runWorkload.Spec.MemoryMiB, runWorkload.Spec.DiskMiB),
			LivenessProbe:  livenessProbe,
			ReadinessProbe: readinessProbe,
		},
	}

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetStatefulSetName(runWorkload),
			Namespace: runWorkload.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			PodManagementPolicy: "Parallel",
			Replicas:            &runWorkload.Spec.Instances,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers:       containers,
					ImagePullSecrets: runWorkload.Spec.ImagePullSecrets,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
					},
				},
			},
		},
	}

	automountServiceAccountToken := false
	statefulSet.Spec.Template.Spec.AutomountServiceAccountToken = &automountServiceAccountToken
	statefulSet.Spec.Selector = StatefulSetLabelSelector(&runWorkload)

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

	err := controllerutil.SetOwnerReference(&runWorkload, statefulSet, r.Scheme)
	if err != nil {
		r.Log.Error(err, "failed to set OwnerRef on StatefulSet")
		return nil, err
	}

	labels := map[string]string{
		LabelGUID:            runWorkload.Spec.GUID,
		LabelProcessType:     runWorkload.Spec.ProcessType,
		LabelVersion:         runWorkload.Spec.Version,
		LabelAppGUID:         runWorkload.Spec.AppGUID,
		LabelRunWorkloadGUID: runWorkload.Name,
	}

	statefulSet.Spec.Template.Labels = labels
	statefulSet.Labels = labels

	annotations := map[string]string{
		AnnotationAppID:       runWorkload.Spec.AppGUID,
		AnnotationVersion:     runWorkload.Spec.Version,
		AnnotationProcessGUID: fmt.Sprintf("%s-%s", runWorkload.Spec.GUID, runWorkload.Spec.Version),
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

func MebibyteQuantity(miB int64) resource.Quantity {
	memory := resource.Quantity{
		Format: resource.BinarySI,
	}
	//nolint:gomnd
	memory.Set(miB * 1024 * 1024)

	return memory
}

func StatefulSetLabelSelector(runWorkload *korifiv1alpha1.RunWorkload) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			LabelGUID:    runWorkload.Spec.GUID,
			LabelVersion: runWorkload.Spec.Version,
		},
	}
}

func getContainerResources(cpuWeight uint8, memoryMiB, diskMiB int64) corev1.ResourceRequirements {
	memory := MebibyteQuantity(memoryMiB)
	cpu := ToCPUMillicores(cpuWeight)
	ephemeralStorage := MebibyteQuantity(diskMiB)

	return corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory:           memory,
			corev1.ResourceEphemeralStorage: ephemeralStorage,
		},
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: memory,
			corev1.ResourceCPU:    cpu,
		},
	}
}

func ToCPUMillicores(cpuPercentage uint8) resource.Quantity {
	return *resource.NewScaledQuantity(int64(cpuPercentage), resource.Milli)
}

func CreateLivenessProbe(runWorkload korifiv1alpha1.RunWorkload) *corev1.Probe {
	initialDelay := toSeconds(runWorkload.Spec.Health.TimeoutMs)

	if runWorkload.Spec.Health.Type == "http" {
		return createHTTPProbe(runWorkload, initialDelay, LivenessFailureThreshold)
	}

	if runWorkload.Spec.Health.Type == "port" {
		return createPortProbe(runWorkload, initialDelay, LivenessFailureThreshold)
	}

	return nil
}

func CreateReadinessProbe(runWorkload korifiv1alpha1.RunWorkload) *corev1.Probe {
	if runWorkload.Spec.Health.Type == "http" {
		return createHTTPProbe(runWorkload, 0, ReadinessFailureThreshold)
	}

	if runWorkload.Spec.Health.Type == "port" {
		return createPortProbe(runWorkload, 0, ReadinessFailureThreshold)
	}

	return nil
}

func createPortProbe(runWorkload korifiv1alpha1.RunWorkload, initialDelay, failureThreshold int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: tcpSocketAction(runWorkload),
		},
		InitialDelaySeconds: initialDelay,
		FailureThreshold:    failureThreshold,
	}
}

func createHTTPProbe(runWorkload korifiv1alpha1.RunWorkload, initialDelay, failureThreshold int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: httpGetAction(runWorkload),
		},
		InitialDelaySeconds: initialDelay,
		FailureThreshold:    failureThreshold,
	}
}

func httpGetAction(runWorkload korifiv1alpha1.RunWorkload) *corev1.HTTPGetAction {
	return &corev1.HTTPGetAction{
		Path: runWorkload.Spec.Health.Endpoint,
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: runWorkload.Spec.Health.Port},
	}
}

func tcpSocketAction(runWorkload korifiv1alpha1.RunWorkload) *corev1.TCPSocketAction {
	return &corev1.TCPSocketAction{
		Port: intstr.IntOrString{Type: intstr.Int, IntVal: runWorkload.Spec.Health.Port},
	}
}

func toSeconds(millis uint) int32 {
	return int32(millis / 1000) //nolint:gomnd
}
