package jobs

import (
	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/utils"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	batch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	taskContainerName = "opi-task"
	parallelism       = 1
	completions       = 1
)

type Converter struct {
	serviceAccountName                string
	registrySecretName                string
	allowAutomountServiceAccountToken bool
}

func NewTaskToJobConverter(
	serviceAccountName string,
	registrySecretName string,
	allowAutomountServiceAccountToken bool,
) *Converter {
	return &Converter{
		serviceAccountName:                serviceAccountName,
		registrySecretName:                registrySecretName,
		allowAutomountServiceAccountToken: allowAutomountServiceAccountToken,
	}
}

func (m *Converter) Convert(task *eiriniv1.Task, privateRegistrySecret *corev1.Secret) *batch.Job {
	job := m.toJob(task)
	job.Spec.Template.Spec.ServiceAccountName = m.serviceAccountName
	job.Labels[LabelSourceType] = TaskSourceType
	job.Labels[LabelName] = task.Spec.Name
	job.Spec.Template.Annotations[AnnotationGUID] = task.Spec.GUID
	job.Spec.Template.Annotations[AnnotationTaskContainerName] = taskContainerName

	envs := getEnvs(task)
	envs = append(envs, task.Spec.Environment...)
	containers := []corev1.Container{
		{
			Name:            taskContainerName,
			Image:           task.Spec.Image,
			ImagePullPolicy: corev1.PullAlways,
			Env:             envs,
			Command:         task.Spec.Command,
		},
	}

	job.Spec.Template.Spec.ImagePullSecrets = []corev1.LocalObjectReference{
		{
			Name: m.registrySecretName,
		},
	}

	if privateRegistrySecret != nil {
		job.Spec.Template.Spec.ImagePullSecrets = append(job.Spec.Template.Spec.ImagePullSecrets,
			corev1.LocalObjectReference{Name: privateRegistrySecret.Name})
	}

	job.Spec.Template.Spec.Containers = containers

	return job
}

func (m *Converter) toJob(task *eiriniv1.Task) *batch.Job {
	runAsNonRoot := true

	job := &batch.Job{
		Spec: batch.JobSpec{
			Parallelism:  int32ptr(parallelism),
			Completions:  int32ptr(completions),
			BackoffLimit: int32ptr(0),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
					},
				},
			},
		},
	}

	if !m.allowAutomountServiceAccountToken {
		automountServiceAccountToken := false
		job.Spec.Template.Spec.AutomountServiceAccountToken = &automountServiceAccountToken
	}

	job.Name = utils.GetJobName(task)

	job.Labels = map[string]string{
		LabelGUID:    task.Spec.GUID,
		LabelAppGUID: task.Spec.AppGUID,
	}

	job.Annotations = map[string]string{
		AnnotationAppName:              task.Spec.AppName,
		AnnotationAppID:                task.Spec.AppGUID,
		AnnotationOrgName:              task.Spec.OrgName,
		AnnotationOrgGUID:              task.Spec.OrgGUID,
		AnnotationSpaceName:            task.Spec.SpaceName,
		AnnotationSpaceGUID:            task.Spec.SpaceGUID,
		corev1.SeccompPodAnnotationKey: corev1.SeccompProfileRuntimeDefault,
	}

	job.Spec.Template.Labels = job.Labels
	job.Spec.Template.Annotations = job.Annotations

	return job
}

func getEnvs(task *eiriniv1.Task) []corev1.EnvVar {
	envs := utils.MapToEnvVar(task.Spec.Env)
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
		{Name: eirinictrl.EnvCFInstanceAddr, Value: ""},
		{Name: eirinictrl.EnvCFInstancePort, Value: ""},
		{Name: eirinictrl.EnvCFInstancePorts, Value: "[]"},
	}

	envs = append(envs, fieldEnvs...)

	return envs
}

func int32ptr(i int) *int32 {
	u := int32(i)

	return &u
}
