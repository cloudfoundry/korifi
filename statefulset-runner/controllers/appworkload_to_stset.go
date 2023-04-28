package controllers

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
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
	nameSuffix, err := hash(fmt.Sprintf("%s-%s", appWorkload.Spec.GUID, appWorkload.Spec.Version))
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

	ports := []corev1.ContainerPort{}

	for _, port := range appWorkload.Spec.Ports {
		ports = append(ports, corev1.ContainerPort{ContainerPort: port})
	}

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
					},
					ServiceAccountName: ServiceAccountName,
				},
			},
		},
	}

	statefulSet.Spec.Template.Spec.AutomountServiceAccountToken = tools.PtrTo(false)
	statefulSet.Spec.Selector = statefulSetLabelSelector(appWorkload)

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

	err = controllerutil.SetOwnerReference(appWorkload, statefulSet, r.scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to set OwnerRef on StatefulSet :%w", err)
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
	if restartedAt, hasRestartedAt := appWorkload.Annotations["korifi.cloudfoundry.org/restartedAt"]; hasRestartedAt {
		annotations["korifi.cloudfoundry.org/restartedAt"] = restartedAt
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

func toLabelSelectorRequirements(selector *metav1.LabelSelector) []metav1.LabelSelectorRequirement {
	labels := make([]string, 0, len(selector.MatchLabels))
	for k := range selector.MatchLabels {
		labels = append(labels, k)
	}
	sort.Strings(labels)

	reqs := make([]metav1.LabelSelectorRequirement, 0, len(labels))
	for _, label := range labels {
		reqs = append(reqs, metav1.LabelSelectorRequirement{
			Key:      label,
			Operator: metav1.LabelSelectorOpIn,
			Values:   []string{selector.MatchLabels[label]},
		})
	}

	return reqs
}

func statefulSetLabelSelector(appWorkload *korifiv1alpha1.AppWorkload) *metav1.LabelSelector {
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
