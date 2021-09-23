package testutils

import (
	"fmt"
	"k8s.io/apimachinery/pkg/api/meta"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GenerateGUID() string {
	newUUID, err := uuid.NewUUID()
	if err != nil {
		errorMessage := fmt.Sprintf("could not generate a UUID %v", err)
		panic(errorMessage)
	}
	return newUUID.String()
}

func InitializeAppCR(appGUID string, spaceGUID string) *workloadsv1alpha1.CFApp {
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
		},
	}
}

func InitializePackageCR(packageGUID, namespaceGUID, appGUID string) *workloadsv1alpha1.CFPackage {
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
					ImagePullSecrets: nil,
				},
			},
		},
	}
}

func InitializeCFBuild(cfBuildGUID string, namespace string, cfPackageGUID string, cfAppGUID string) *workloadsv1alpha1.CFBuild {
	return &workloadsv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfBuildGUID,
			Namespace: namespace,
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

func SetStatusCondition(conditions *[]metav1.Condition, conditionType string, status metav1.ConditionStatus) {
	meta.SetStatusCondition(conditions, metav1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  "reasons",
		Message: "",
	})
}
