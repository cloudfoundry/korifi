package testutils

import (
	"encoding/base64"

	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"

	"k8s.io/apimachinery/pkg/api/meta"

	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CFAppLabelKey         = "korifi.cloudfoundry.org/app-guid"
	cfAppRevisionKey      = "korifi.cloudfoundry.org/app-rev"
	CFProcessGUIDLabelKey = "korifi.cloudfoundry.org/process-guid"
	CFProcessTypeLabelKey = "korifi.cloudfoundry.org/process-type"
	appFinalizerName      = "cfApp.korifi.cloudfoundry.org"
)

func GenerateGUID() string {
	return uuid.NewString()
}

func PrefixedGUID(prefix string) string {
	return prefix + "-" + uuid.NewString()[:8]
}

func BuildNamespaceObject(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func BuildSubNamespaceAnchorObject(hnsGUID string, spaceGUID string) *v1alpha2.SubnamespaceAnchor {
	return &v1alpha2.SubnamespaceAnchor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hnsGUID,
			Namespace: spaceGUID,
		},
	}
}

func BuildCFAppCRObject(appGUID string, spaceGUID string) *v1alpha1.CFApp {
	return &v1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appGUID,
			Namespace: spaceGUID,
			Annotations: map[string]string{
				cfAppRevisionKey: "0",
			},
			Finalizers: []string{
				appFinalizerName,
			},
		},
		Spec: v1alpha1.CFAppSpec{
			DisplayName:  "test-app-name",
			DesiredState: "STOPPED",
			Lifecycle: v1alpha1.Lifecycle{
				Type: "buildpack",
				Data: v1alpha1.LifecycleData{
					Buildpacks: []string{},
					Stack:      "",
				},
			},
			EnvSecretName: appGUID + "-env",
		},
	}
}

func BuildCFOrgObject(orgGUID string, spaceGUID string) *v1alpha1.CFOrg {
	return &v1alpha1.CFOrg{
		ObjectMeta: metav1.ObjectMeta{
			Name:      orgGUID,
			Namespace: spaceGUID,
		},
		Spec: v1alpha1.CFOrgSpec{
			DisplayName: "test-org-name",
		},
	}
}

func BuildCFSpaceObject(spaceGUID string, orgGUID string) *v1alpha1.CFSpace {
	return &v1alpha1.CFSpace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spaceGUID,
			Namespace: orgGUID,
		},
		Spec: v1alpha1.CFSpaceSpec{
			DisplayName: "test-space-name",
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

func BuildCFPackageCRObject(packageGUID, namespaceGUID, appGUID string) *v1alpha1.CFPackage {
	return &v1alpha1.CFPackage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      packageGUID,
			Namespace: namespaceGUID,
		},
		Spec: v1alpha1.CFPackageSpec{
			Type: "bits",
			AppRef: corev1.LocalObjectReference{
				Name: appGUID,
			},
			Source: v1alpha1.PackageSource{
				Registry: v1alpha1.Registry{
					Image:            "PACKAGE_IMAGE",
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "source-registry-image-pull-secret"}},
				},
			},
		},
	}
}

func BuildCFBuildObject(cfBuildGUID string, namespace string, cfPackageGUID string, cfAppGUID string) *v1alpha1.CFBuild {
	return &v1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfBuildGUID,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "CFBuild",
			APIVersion: "korifi.cloudfoundry.org/v1alpha1",
		},
		Spec: v1alpha1.CFBuildSpec{
			PackageRef: corev1.LocalObjectReference{
				Name: cfPackageGUID,
			},
			AppRef: corev1.LocalObjectReference{
				Name: cfAppGUID,
			},
			StagingMemoryMB: 1024,
			StagingDiskMB:   1024,
			Lifecycle: v1alpha1.Lifecycle{
				Type: "buildpack",
				Data: v1alpha1.LifecycleData{
					Buildpacks: nil,
					Stack:      "",
				},
			},
		},
	}
}

func BuildCFBuildDropletStatusObject(dropletProcessTypeMap map[string]string, dropletPorts []int32) *v1alpha1.BuildDropletStatus {
	dropletProcessTypes := make([]v1alpha1.ProcessType, 0, len(dropletProcessTypeMap))
	for k, v := range dropletProcessTypeMap {
		dropletProcessTypes = append(dropletProcessTypes, v1alpha1.ProcessType{
			Type:    k,
			Command: v,
		})
	}
	return &v1alpha1.BuildDropletStatus{
		Registry: v1alpha1.Registry{
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
		Secrets:          []corev1.ObjectReference{{Name: imagePullSecretName}},
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: imagePullSecretName}},
	}
}

func BuildCFProcessCRObject(cfProcessGUID string, namespace string, cfAppGUID string, processType string, processCommand string) *v1alpha1.CFProcess {
	return &v1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfProcessGUID,
			Namespace: namespace,
			Labels: map[string]string{
				CFAppLabelKey:         cfAppGUID,
				CFProcessGUIDLabelKey: cfProcessGUID,
				CFProcessTypeLabelKey: processType,
			},
		},
		Spec: v1alpha1.CFProcessSpec{
			AppRef:      corev1.LocalObjectReference{Name: cfAppGUID},
			ProcessType: processType,
			Command:     processCommand,
			HealthCheck: v1alpha1.HealthCheck{
				Type: "process",
				Data: v1alpha1.HealthCheckData{
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

func UpdateCFBuildWithDropletStatus(cfbuild *v1alpha1.CFBuild) {
	cfbuild.Status.Droplet = &v1alpha1.BuildDropletStatus{
		Registry: v1alpha1.Registry{
			Image:            "my-image",
			ImagePullSecrets: nil,
		},
		Stack: "cflinuxfs3",
		ProcessTypes: []v1alpha1.ProcessType{
			{
				Type:    "web",
				Command: "web-command",
			},
		},
		Ports: []int32{8080},
	}
}

func UpdateCFAppWithCurrentDropletRef(cfApp *v1alpha1.CFApp, buildGUID string) {
	cfApp.Spec.CurrentDropletRef = corev1.LocalObjectReference{Name: buildGUID}
}
