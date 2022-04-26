package repositories_test

import (
	"context"

	servicesv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/services/v1alpha1"

	. "code.cloudfoundry.org/korifi/api/repositories"
	workloadsv1alpha1 "code.cloudfoundry.org/korifi/controllers/apis/workloads/v1alpha1"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			DisplayName:  appName,
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

func createPackageCR(ctx context.Context, k8sClient client.Client, packageGUID, appGUID, spaceGUID, srcRegistryImage string) *workloadsv1alpha1.CFPackage {
	toReturn := &workloadsv1alpha1.CFPackage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      packageGUID,
			Namespace: spaceGUID,
		},
		Spec: workloadsv1alpha1.CFPackageSpec{
			Type: "bits",
			AppRef: corev1.LocalObjectReference{
				Name: appGUID,
			},
		},
	}

	if srcRegistryImage != "" {
		toReturn.Spec.Source.Registry.Image = srcRegistryImage
	}

	Expect(
		k8sClient.Create(ctx, toReturn),
	).To(Succeed())
	return toReturn
}

func createBuild(ctx context.Context, k8sClient client.Client, namespace, buildGUID, packageGUID, appGUID string) *workloadsv1alpha1.CFBuild {
	const (
		stagingMemory = 1024
		stagingDisk   = 2048
	)

	record := &workloadsv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildGUID,
			Namespace: namespace,
			Labels: map[string]string{
				workloadsv1alpha1.CFAppGUIDLabelKey: appGUID,
			},
		},
		Spec: workloadsv1alpha1.CFBuildSpec{
			PackageRef: corev1.LocalObjectReference{
				Name: packageGUID,
			},
			AppRef: corev1.LocalObjectReference{
				Name: appGUID,
			},
			StagingMemoryMB: stagingMemory,
			StagingDiskMB:   stagingDisk,
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
		k8sClient.Create(ctx, record),
	).To(Succeed())
	return record
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

func createServiceInstanceCR(ctx context.Context, k8sClient client.Client, serviceInstanceGUID, spaceGUID, name, secretName string) *servicesv1alpha1.CFServiceInstance {
	toReturn := &servicesv1alpha1.CFServiceInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceInstanceGUID,
			Namespace: spaceGUID,
		},
		Spec: servicesv1alpha1.CFServiceInstanceSpec{
			DisplayName: name,
			SecretName:  secretName,
			Type:        "user-provided",
		},
	}
	Expect(
		k8sClient.Create(ctx, toReturn),
	).To(Succeed())
	return toReturn
}

func createServiceBindingCR(ctx context.Context, k8sClient client.Client, serviceBindingGUID, spaceGUID string, name *string, serviceInstanceName, appName string) *servicesv1alpha1.CFServiceBinding {
	toReturn := &servicesv1alpha1.CFServiceBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceBindingGUID,
			Namespace: spaceGUID,
		},
		Spec: servicesv1alpha1.CFServiceBindingSpec{
			DisplayName: name,
			Service: corev1.ObjectReference{
				Kind:       "ServiceInstance",
				Name:       serviceInstanceName,
				APIVersion: "services.cloudfoundry.org/v1alpha1",
			},
			AppRef: corev1.LocalObjectReference{
				Name: appName,
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
