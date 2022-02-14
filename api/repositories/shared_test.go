package repositories_test

import (
	"context"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	cfAppGUIDLabelKey = "workloads.cloudfoundry.org/app-guid"
)

func generateGUID() string {
	return uuid.NewString()
}

func prefixedGUID(prefix string) string {
	return prefix + "-" + uuid.NewString()[:8]
}

func createAppCR(ctx context.Context, k8sClient client.Client, appName, appGUID, spaceGUID, desiredState string) *workloadsv1alpha1.CFApp {
	toReturn := &workloadsv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appGUID,
			Namespace: spaceGUID,
		},
		Spec: workloadsv1alpha1.CFAppSpec{
			Name:         appName,
			DesiredState: workloadsv1alpha1.DesiredState(desiredState),
			Lifecycle: workloadsv1alpha1.Lifecycle{
				Type: "buildpack",
				Data: workloadsv1alpha1.LifecycleData{
					Buildpacks: []string{},
					Stack:      "",
				},
			},
		},
	}
	Expect(
		k8sClient.Create(ctx, toReturn),
	).To(Succeed())
	return toReturn
}

func createProcessCR(ctx context.Context, k8sClient client.Client, processGUID, spaceGUID, appGUID string) *workloadsv1alpha1.CFProcess {
	toReturn := &workloadsv1alpha1.CFProcess{
		ObjectMeta: metav1.ObjectMeta{
			Name:      processGUID,
			Namespace: spaceGUID,
			Labels: map[string]string{
				cfAppGUIDLabelKey: appGUID,
			},
		},
		Spec: workloadsv1alpha1.CFProcessSpec{
			AppRef: corev1.LocalObjectReference{
				Name: appGUID,
			},
			ProcessType: "web",
			Command:     "",
			HealthCheck: workloadsv1alpha1.HealthCheck{
				Type: "process",
				Data: workloadsv1alpha1.HealthCheckData{
					InvocationTimeoutSeconds: 0,
					TimeoutSeconds:           0,
				},
			},
			DesiredInstances: 1,
			MemoryMB:         500,
			DiskQuotaMB:      512,
			Ports:            []int32{8080},
		},
	}
	Expect(
		k8sClient.Create(ctx, toReturn),
	).To(Succeed())
	return toReturn
}

func createDropletCR(ctx context.Context, k8sClient client.Client, dropletGUID, appGUID, spaceGUID string) *workloadsv1alpha1.CFBuild {
	toReturn := &workloadsv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dropletGUID,
			Namespace: spaceGUID,
		},
		Spec: workloadsv1alpha1.CFBuildSpec{
			AppRef: corev1.LocalObjectReference{Name: appGUID},
			Lifecycle: workloadsv1alpha1.Lifecycle{
				Type: "buildpack",
			},
		},
	}
	Expect(
		k8sClient.Create(ctx, toReturn),
	).To(Succeed())
	return toReturn
}

func initializeAppCreateMessage(appName string, spaceGUID string) CreateAppMessage {
	return CreateAppMessage{
		Name:      appName,
		SpaceGUID: spaceGUID,
		State:     "STOPPED",
		Lifecycle: Lifecycle{
			Type: "buildpack",
			Data: LifecycleData{
				Buildpacks: []string{},
				Stack:      "cflinuxfs3",
			},
		},
	}
}

func generateAppEnvSecretName(appGUID string) string {
	return appGUID + "-env"
}
