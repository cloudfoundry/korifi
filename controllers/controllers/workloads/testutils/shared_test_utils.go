package testutils

import (
	"encoding/base64"

	"k8s.io/apimachinery/pkg/api/meta"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CFAppLabelKey         = "workloads.cloudfoundry.org/app-guid"
	CFProcessGUIDLabelKey = "workloads.cloudfoundry.org/process-guid"
	CFProcessTypeLabelKey = "workloads.cloudfoundry.org/process-type"
)

func GenerateGUID() string {
	return uuid.NewString()
}

func BuildNamespaceObject(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func BuildCFAppCRObject(appGUID string, spaceGUID string) *workloadsv1alpha1.CFApp {
	return &workloadsv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appGUID,
			Namespace: spaceGUID,
		},
		Spec: workloadsv1alpha1.CFAppSpec{
			Name:         "test-app-name",
			DesiredState: "STOPPED",
			Lifecycle: workloadsv1alpha1.Lifecycle{
				Type: "buildpack",
				Data: workloadsv1alpha1.LifecycleData{
					Buildpacks: []string{},
					Stack:      "",
				},
			},
			EnvSecretName: "test-env-secret-name",
		},
	}
}

func BuildCFAppEnvVarsSecret(appGUID, spaceGUID string, envVars map[string]string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: spaceGUID,
			Name:      appGUID + "-env",
		},
		StringData: envVars,
	}
}

func BuildCFPackageCRObject(packageGUID, namespaceGUID, appGUID string) *workloadsv1alpha1.CFPackage {
	return &workloadsv1alpha1.CFPackage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      packageGUID,
			Namespace: namespaceGUID,
		},
		Spec: workloadsv1alpha1.CFPackageSpec{
			Type: "bits",
			AppRef: corev1.LocalObjectReference{
				Name: appGUID,
			},
			Source: workloadsv1alpha1.PackageSource{
				Registry: workloadsv1alpha1.Registry{
					Image:            "PACKAGE_IMAGE",
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "source-registry-image-pull-secret"}},
				},
			},
		},
	}
}

func BuildCFBuildObject(cfBuildGUID string, namespace string, cfPackageGUID string, cfAppGUID string) *workloadsv1alpha1.CFBuild {
	return &workloadsv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfBuildGUID,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "CFBuild",
			APIVersion: "workloads.cloudfoundry.org/v1alpha1",
		},
		Spec: workloadsv1alpha1.CFBuildSpec{
			PackageRef: corev1.LocalObjectReference{
				Name: cfPackageGUID,
			},
			AppRef: corev1.LocalObjectReference{
				Name: cfAppGUID,
			},
			StagingMemoryMB: 1024,
			StagingDiskMB:   1024,
			Lifecycle: workloadsv1alpha1.Lifecycle{
				Type: "buildpack",
				Data: workloadsv1alpha1.LifecycleData{
					Buildpacks: nil,
					Stack:      "",
				},
			},
		},
	}
}

func BuildCFBuildDropletStatusObject(dropletProcessTypeMap map[string]string, dropletPorts []int32) *workloadsv1alpha1.BuildDropletStatus {
	dropletProcessTypes := make([]workloadsv1alpha1.ProcessType, 0, len(dropletProcessTypeMap))
	for k, v := range dropletProcessTypeMap {
		dropletProcessTypes = append(dropletProcessTypes, workloadsv1alpha1.ProcessType{
			Type:    k,
			Command: v,
		})
	}
	return &workloadsv1alpha1.BuildDropletStatus{
		Registry: workloadsv1alpha1.Registry{
			Image:            "image/registry/url",
			ImagePullSecrets: nil,
		},
		Stack:        "cflinuxfs3",
		ProcessTypes: dropletProcessTypes,
		Ports:        dropletPorts,
	}
}

func BuildDockerRegistrySecret(name, namespace string) *corev1.Secret {
	dockerRegistryUsername := "user"
	dockerRegistryPassword := "password"
	dockerAuth := base64.StdEncoding.EncodeToString([]byte(dockerRegistryUsername + ":" + dockerRegistryPassword))
	dockerConfigJSON := `{"auths":{"https://index.docker.io/v1/":{"username":"` + dockerRegistryUsername + `","password":"` + dockerRegistryPassword + `","auth":"` + dockerAuth + `"}}}`
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Immutable: nil,
		Data:      nil,
		StringData: map[string]string{
			".dockerconfigjson": dockerConfigJSON,
		},
		Type: "kubernetes.io/dockerconfigjson",
	}
}

func BuildServiceAccount(name, namespace, imagePullSecretName string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Secrets:          []corev1.ObjectReference{corev1.ObjectReference{Name: imagePullSecretName}},
		ImagePullSecrets: []corev1.LocalObjectReference{corev1.LocalObjectReference{Name: imagePullSecretName}},
	}
}

func BuildCFProcessCRObject(cfProcessGUID string, namespace string, cfAppGUID string, processType string, processCommand string) *workloadsv1alpha1.CFProcess {
	return &workloadsv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfProcessGUID,
			Namespace: namespace,
			Labels: map[string]string{
				CFAppLabelKey:         cfAppGUID,
				CFProcessGUIDLabelKey: cfProcessGUID,
				CFProcessTypeLabelKey: processType,
			},
		},
		Spec: workloadsv1alpha1.CFProcessSpec{
			AppRef:      corev1.LocalObjectReference{Name: cfAppGUID},
			ProcessType: processType,
			Command:     processCommand,
			HealthCheck: workloadsv1alpha1.HealthCheck{
				Type: "process",
				Data: workloadsv1alpha1.HealthCheckData{
					InvocationTimeoutSeconds: 0,
					TimeoutSeconds:           0,
				},
			},
			DesiredInstances: 0,
			MemoryMB:         100,
			DiskQuotaMB:      100,
			Ports:            []int32{8080},
		},
	}
}

func SetStatusCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  "reasons",
		Message: "",
	})
}

func UpdateCFBuildWithDropletStatus(cfbuild *workloadsv1alpha1.CFBuild) {
	cfbuild.Status.BuildDropletStatus = &workloadsv1alpha1.BuildDropletStatus{
		Registry: workloadsv1alpha1.Registry{
			Image:            "my-image",
			ImagePullSecrets: nil,
		},
		Stack: "cflinuxfs3",
		ProcessTypes: []workloadsv1alpha1.ProcessType{
			{
				Type:    "web",
				Command: "web-command",
			},
		},
		Ports: []int32{8080},
	}
}

func UpdateCFAppWithCurrentDropletRef(cfApp *workloadsv1alpha1.CFApp, buildGUID string) {
	cfApp.Spec.CurrentDropletRef = corev1.LocalObjectReference{Name: buildGUID}
}
