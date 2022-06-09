package stset

import (
	"fmt"

	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/utils"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//counterfeiter:generate . ProbeCreator

const PodAffinityTermWeight = 100

type ProbeCreator func(lrp *eiriniv1.LRP) *corev1.Probe

type LRPToStatefulSet struct {
	applicationServiceAccount         string
	registrySecretName                string
	allowAutomountServiceAccountToken bool
	allowRunImageAsRoot               bool
	livenessProbeCreator              ProbeCreator
	readinessProbeCreator             ProbeCreator
}

func NewLRPToStatefulSetConverter(
	applicationServiceAccount string,
	registrySecretName string,
	allowAutomountServiceAccountToken bool,
	allowRunImageAsRoot bool,
	livenessProbeCreator ProbeCreator,
	readinessProbeCreator ProbeCreator,
) *LRPToStatefulSet {
	return &LRPToStatefulSet{
		applicationServiceAccount:         applicationServiceAccount,
		registrySecretName:                registrySecretName,
		allowAutomountServiceAccountToken: allowAutomountServiceAccountToken,
		allowRunImageAsRoot:               allowRunImageAsRoot,
		livenessProbeCreator:              livenessProbeCreator,
		readinessProbeCreator:             readinessProbeCreator,
	}
}

func (c *LRPToStatefulSet) Convert(statefulSetName string, lrp *eiriniv1.LRP, privateRegistrySecret *corev1.Secret) (*appsv1.StatefulSet, error) {
	envs := utils.MapToEnvVar(lrp.Spec.Env)
	envs = append(envs, lrp.Spec.Environment...)
	fieldEnvs := []corev1.EnvVar{
		{
			Name: eirinictrl.EnvPodName,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name: eirinictrl.EnvCFInstanceGUID,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.uid",
				},
			},
		},
		{
			Name: eirinictrl.EnvCFInstanceIP,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.hostIP",
				},
			},
		},
		{
			Name: eirinictrl.EnvCFInstanceInternalIP,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
	}

	envs = append(envs, fieldEnvs...)
	ports := []corev1.ContainerPort{}

	for _, port := range lrp.Spec.Ports {
		ports = append(ports, corev1.ContainerPort{ContainerPort: port})
	}

	livenessProbe := c.livenessProbeCreator(lrp)
	readinessProbe := c.readinessProbeCreator(lrp)

	volumes, volumeMounts := getVolumeSpecs(lrp.Spec.VolumeMounts)
	allowPrivilegeEscalation := false
	imagePullSecrets := c.calculateImagePullSecrets(privateRegistrySecret)

	containers := []corev1.Container{
		{
			Name:            ApplicationContainerName,
			Image:           lrp.Spec.Image,
			ImagePullPolicy: corev1.PullAlways,
			Command:         lrp.Spec.Command,
			Env:             envs,
			Ports:           ports,
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: &allowPrivilegeEscalation,
			},
			Resources:      getContainerResources(lrp.Spec.CPUWeight, lrp.Spec.MemoryMB, lrp.Spec.DiskMB),
			LivenessProbe:  livenessProbe,
			ReadinessProbe: readinessProbe,
			VolumeMounts:   volumeMounts,
		},
	}

	sidecarContainers := getSidecarContainers(lrp)
	containers = append(containers, sidecarContainers...)
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: statefulSetName,
		},
		Spec: appsv1.StatefulSetSpec{
			PodManagementPolicy: "Parallel",
			Replicas:            int32ptr(lrp.Spec.Instances),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers:         containers,
					ImagePullSecrets:   imagePullSecrets,
					SecurityContext:    c.getGetSecurityContext(),
					ServiceAccountName: c.applicationServiceAccount,
					Volumes:            volumes,
				},
			},
		},
	}

	if !c.allowAutomountServiceAccountToken {
		automountServiceAccountToken := false
		statefulSet.Spec.Template.Spec.AutomountServiceAccountToken = &automountServiceAccountToken
	}

	statefulSet.Spec.Selector = StatefulSetLabelSelector(lrp)

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

	labels := map[string]string{
		LabelOrgGUID:     lrp.Spec.OrgGUID,
		LabelOrgName:     lrp.Spec.OrgName,
		LabelSpaceGUID:   lrp.Spec.SpaceGUID,
		LabelSpaceName:   lrp.Spec.SpaceName,
		LabelGUID:        lrp.Spec.GUID,
		LabelProcessType: lrp.Spec.ProcessType,
		LabelVersion:     lrp.Spec.Version,
		LabelAppGUID:     lrp.Spec.AppGUID,
		LabelSourceType:  AppSourceType,
	}

	statefulSet.Spec.Template.Labels = labels
	statefulSet.Labels = labels

	annotations := map[string]string{
		AnnotationSpaceName:   lrp.Spec.SpaceName,
		AnnotationSpaceGUID:   lrp.Spec.SpaceGUID,
		AnnotationAppID:       lrp.Spec.AppGUID,
		AnnotationVersion:     lrp.Spec.Version,
		AnnotationProcessGUID: fmt.Sprintf("%s-%s", lrp.Spec.GUID, lrp.Spec.Version),
		AnnotationAppName:     lrp.Spec.AppName,
		AnnotationOrgName:     lrp.Spec.OrgName,
		AnnotationOrgGUID:     lrp.Spec.OrgGUID,
	}

	for k, v := range lrp.Spec.UserDefinedAnnotations {
		annotations[k] = v
	}

	statefulSet.Annotations = annotations
	statefulSet.Spec.Template.Annotations = annotations
	statefulSet.Spec.Template.Annotations[corev1.SeccompPodAnnotationKey] = corev1.SeccompProfileRuntimeDefault

	return statefulSet, nil
}

func (c *LRPToStatefulSet) calculateImagePullSecrets(privateRegistrySecret *corev1.Secret) []corev1.LocalObjectReference {
	imagePullSecrets := []corev1.LocalObjectReference{
		{Name: c.registrySecretName},
	}

	if privateRegistrySecret != nil {
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{
			Name: privateRegistrySecret.Name,
		})
	}

	return imagePullSecrets
}

func (c *LRPToStatefulSet) getGetSecurityContext() *corev1.PodSecurityContext {
	if c.allowRunImageAsRoot {
		return nil
	}

	runAsNonRoot := true

	return &corev1.PodSecurityContext{
		RunAsNonRoot: &runAsNonRoot,
	}
}

func getVolumeSpecs(lrpVolumeMounts []eiriniv1.VolumeMount) ([]corev1.Volume, []corev1.VolumeMount) {
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}

	for _, vm := range lrpVolumeMounts {
		volumes = append(volumes, corev1.Volume{
			Name: vm.ClaimName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: vm.ClaimName,
				},
			},
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      vm.ClaimName,
			MountPath: vm.MountPath,
		})
	}

	return volumes, volumeMounts
}

func getContainerResources(cpuWeight uint8, memoryMB, diskMB int64) corev1.ResourceRequirements {
	memory := MebibyteQuantity(memoryMB)
	cpu := toCPUMillicores(cpuWeight)
	ephemeralStorage := MebibyteQuantity(diskMB)

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

func MebibyteQuantity(miB int64) resource.Quantity {
	memory := resource.Quantity{
		Format: resource.BinarySI,
	}
	//nolint:gomnd
	memory.Set(miB * 1024 * 1024)

	return memory
}

func toCPUMillicores(cpuPercentage uint8) resource.Quantity {
	return *resource.NewScaledQuantity(int64(cpuPercentage), resource.Milli)
}

func getSidecarContainers(lrp *eiriniv1.LRP) []corev1.Container {
	containers := []corev1.Container{}

	for _, s := range lrp.Spec.Sidecars {
		c := corev1.Container{
			Name:      s.Name,
			Command:   s.Command,
			Image:     lrp.Spec.Image,
			Env:       utils.MapToEnvVar(s.Env),
			Resources: getContainerResources(lrp.Spec.CPUWeight, s.MemoryMB, lrp.Spec.DiskMB),
		}
		containers = append(containers, c)
	}

	return containers
}

func int32ptr(i int) *int32 {
	u := int32(i)

	return &u
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

func StatefulSetLabelSelector(lrp *eiriniv1.LRP) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			LabelGUID:       lrp.Spec.GUID,
			LabelVersion:    lrp.Spec.Version,
			LabelSourceType: AppSourceType,
		},
	}
}
