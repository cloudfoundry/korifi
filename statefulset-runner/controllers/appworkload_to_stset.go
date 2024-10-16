package controllers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/v2/it"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

type AppWorkloadToStatefulsetConverter struct {
	scheme *runtime.Scheme
}

func NewAppWorkloadToStatefulsetConverter(scheme *runtime.Scheme) *AppWorkloadToStatefulsetConverter {
	return &AppWorkloadToStatefulsetConverter{
		scheme: scheme,
	}
}

func getStatefulSetName(appWorkload *korifiv1alpha1.AppWorkload) (string, error) {
	lastStopAppRev := appWorkload.Spec.Version
	if annotationVal, ok := appWorkload.Annotations[korifiv1alpha1.CFAppLastStopRevisionKey]; ok {
		lastStopAppRev = annotationVal
	}
	nameSuffix, err := hash(fmt.Sprintf("%s-%s", appWorkload.Spec.GUID, lastStopAppRev))
	if err != nil {
		return "", fmt.Errorf("failed to generate hash for statefulset name: %w", err)
	}

	namePrefix := fmt.Sprintf("%s-%s", appWorkload.Spec.AppGUID, appWorkload.Namespace)
	namePrefix = sanitizeName(namePrefix, appWorkload.Spec.GUID)

	return fmt.Sprintf("%s-%s", namePrefix, nameSuffix), nil
}

func (r *AppWorkloadToStatefulsetConverter) Convert(appWorkload *korifiv1alpha1.AppWorkload) (*appsv1.StatefulSet, error) {
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
			Name: EnvCFInstanceIndex,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fmt.Sprintf("metadata.labels['%s']", korifiv1alpha1.PodIndexLabelKey),
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

	// Sort env vars to guarantee idempotency
	sort.SliceStable(envs, func(i, j int) bool {
		return envs[i].Name < envs[j].Name
	})

	containers := []corev1.Container{
		{
			Name:            ApplicationContainerName,
			Image:           appWorkload.Spec.Image,
			ImagePullPolicy: corev1.PullAlways,
			Command:         appWorkload.Spec.Command,
			Env:             envs,
			Ports: slices.Collect(it.Map(slices.Values(appWorkload.Spec.Ports), func(port int32) corev1.ContainerPort {
				return corev1.ContainerPort{ContainerPort: port}
			})),
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: tools.PtrTo(false),
				Capabilities: &corev1.Capabilities{
					Drop: []corev1.Capability{"ALL"},
				},
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Resources:     appWorkload.Spec.Resources,
			StartupProbe:  appWorkload.Spec.StartupProbe,
			LivenessProbe: appWorkload.Spec.LivenessProbe,
		},
	}

	statefulsetName, err := getStatefulSetName(appWorkload)
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
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					ServiceAccountName: ServiceAccountName,
				},
			},
		},
	}

	statefulSet.Spec.Template.Spec.AutomountServiceAccountToken = tools.PtrTo(false)
	statefulSet.Spec.Selector = statefulSetLabelSelector(appWorkload)

	statefulSet.Spec.Template.Spec.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{
		{
			TopologyKey:       "topology.kubernetes.io/zone",
			MaxSkew:           1,
			WhenUnsatisfiable: "ScheduleAnyway",
			LabelSelector:     statefulSet.Spec.Selector,
			MatchLabelKeys: []string{
				"pod-template-hash",
			},
		},
		{
			TopologyKey:       "kubernetes.io/hostname",
			MaxSkew:           1,
			WhenUnsatisfiable: "ScheduleAnyway",
			LabelSelector:     statefulSet.Spec.Selector,
			MatchLabelKeys: []string{
				"pod-template-hash",
			},
		},
	}

	err = controllerutil.SetControllerReference(appWorkload, statefulSet, r.scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to set OwnerRef on StatefulSet :%w", err)
	}

	labels := map[string]string{
		LabelGUID:            appWorkload.Spec.GUID,
		LabelProcessType:     appWorkload.Spec.ProcessType,
		LabelVersion:         appWorkload.Spec.Version,
		LabelAppGUID:         appWorkload.Spec.AppGUID,
		LabelAppWorkloadGUID: appWorkload.Name,
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

func statefulSetLabelSelector(appWorkload *korifiv1alpha1.AppWorkload) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			LabelGUID: appWorkload.Spec.GUID,
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
