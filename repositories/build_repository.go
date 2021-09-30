package repositories

import (
	"context"
	"errors"
	"fmt"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	BuildStateStaging = "STAGING"
	BuildStateStaged  = "STAGED"
	BuildStateFailed  = "FAILED"

	StagingConditionType   = "Staging"
	SucceededConditionType = "Succeeded"
)

type BuildCreateMessage struct {
	AppGUID         string
	PackageGUID     string
	SpaceGUID       string
	StagingMemoryMB int
	StagingDiskMB   int
	Lifecycle       Lifecycle
	Labels          map[string]string
	Annotations     map[string]string
}

type BuildRecord struct {
	GUID            string
	State           string
	CreatedAt       string
	UpdatedAt       string
	StagingErrorMsg string
	StagingMemoryMB int
	StagingDiskMB   int
	Lifecycle       Lifecycle
	PackageGUID     string
	DropletGUID     string
	AppGUID         string
	Labels          map[string]string
	Annotations     map[string]string
}

type BuildRepo struct {
}

func (b *BuildRepo) FetchBuild(ctx context.Context, k8sClient client.Client, buildGUID string) (BuildRecord, error) {
	// TODO: Could look up namespace from guid => namespace cache to do Get
	buildList := &workloadsv1alpha1.CFBuildList{}
	err := k8sClient.List(ctx, buildList)
	if err != nil { // untested
		return BuildRecord{}, err
	}
	allBuilds := buildList.Items
	matches := b.filterBuildsByMetadataName(allBuilds, buildGUID)

	return b.returnBuild(matches)
}

func (b *BuildRepo) returnBuild(builds []workloadsv1alpha1.CFBuild) (BuildRecord, error) {
	if len(builds) == 0 {
		return BuildRecord{}, NotFoundError{}
	}
	if len(builds) > 1 {
		return BuildRecord{}, errors.New("duplicate builds exist")
	}

	return b.cfBuildToBuildRecord(builds[0]), nil
}

func (b *BuildRepo) cfBuildToBuildRecord(cfBuild workloadsv1alpha1.CFBuild) BuildRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfBuild.ObjectMeta)

	toReturn := BuildRecord{
		GUID:            cfBuild.Name,
		State:           BuildStateStaging,
		CreatedAt:       cfBuild.CreationTimestamp.UTC().Format(TimestampFormat),
		UpdatedAt:       updatedAtTime,
		StagingErrorMsg: "",
		StagingMemoryMB: cfBuild.Spec.StagingMemoryMB,
		StagingDiskMB:   cfBuild.Spec.StagingDiskMB,
		Lifecycle: Lifecycle{
			Type: string(cfBuild.Spec.Lifecycle.Type),
			Data: LifecycleData{
				Buildpacks: []string{},
				Stack:      cfBuild.Spec.Lifecycle.Data.Stack,
			},
		},
		PackageGUID: cfBuild.Spec.PackageRef.Name,
		DropletGUID: "",
		AppGUID:     cfBuild.Spec.AppRef.Name,
		Labels:      cfBuild.Labels,
		Annotations: cfBuild.Annotations,
	}

	if cfBuild.Spec.Lifecycle.Data.Buildpacks != nil {
		toReturn.Lifecycle.Data.Buildpacks = cfBuild.Spec.Lifecycle.Data.Buildpacks
	}

	stagingStatus := getConditionValue(&cfBuild.Status.Conditions, StagingConditionType)
	succeededStatus := getConditionValue(&cfBuild.Status.Conditions, SucceededConditionType)
	// TODO: Consider moving this logic to CRDs repo in case Status Conditions change later?
	if stagingStatus == metav1.ConditionFalse {
		if succeededStatus == metav1.ConditionTrue {
			toReturn.State = BuildStateStaged
			toReturn.DropletGUID = cfBuild.Name
		} else if succeededStatus == metav1.ConditionFalse {
			toReturn.State = BuildStateFailed
			conditionStatus := meta.FindStatusCondition(cfBuild.Status.Conditions, SucceededConditionType)
			toReturn.StagingErrorMsg = fmt.Sprintf("%v: %v", conditionStatus.Reason, conditionStatus.Message)
		}
	}

	return toReturn
}

func (b *BuildRepo) filterBuildsByMetadataName(builds []workloadsv1alpha1.CFBuild, name string) []workloadsv1alpha1.CFBuild {
	var filtered []workloadsv1alpha1.CFBuild
	for i, build := range builds {
		if build.ObjectMeta.Name == name {
			filtered = append(filtered, builds[i])
		}
	}
	return filtered
}

func (b *BuildRepo) CreateBuild(ctx context.Context, k8sClient client.Client, message BuildCreateMessage) (BuildRecord, error) {
	cfBuild := b.buildCreateToCFBuild(message)
	err := k8sClient.Create(ctx, &cfBuild)
	if err != nil { // untested!!!
		return BuildRecord{}, err
	}
	return b.cfBuildToBuildRecord(cfBuild), nil
}

func (b *BuildRepo) buildCreateToCFBuild(message BuildCreateMessage) workloadsv1alpha1.CFBuild {
	guid := uuid.New().String()
	return workloadsv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:        guid,
			Namespace:   message.SpaceGUID,
			Labels:      message.Labels,
			Annotations: message.Annotations,
		},
		Spec: workloadsv1alpha1.CFBuildSpec{
			PackageRef: corev1.LocalObjectReference{
				Name: message.PackageGUID,
			},
			AppRef: corev1.LocalObjectReference{
				Name: message.AppGUID,
			},
			StagingMemoryMB: message.StagingMemoryMB,
			StagingDiskMB:   message.StagingDiskMB,
			Lifecycle: workloadsv1alpha1.Lifecycle{
				Type: workloadsv1alpha1.LifecycleType(message.Lifecycle.Type),
				Data: workloadsv1alpha1.LifecycleData{
					Buildpacks: message.Lifecycle.Data.Buildpacks,
					Stack:      message.Lifecycle.Data.Stack,
				},
			},
		},
	}
}
