package repositories

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// No kubebuilder RBAC tags required, because Build and Droplet are the same CR

type DropletRecord struct {
	GUID            string
	State           string
	CreatedAt       string
	UpdatedAt       string
	DropletErrorMsg string
	Lifecycle       Lifecycle
	Stack           string
	ProcessTypes    map[string]string
	AppGUID         string
	PackageGUID     string
	Labels          map[string]string
	Annotations     map[string]string
}

type DropletRepo struct {
	privilegedClient client.Client
}

func NewDropletRepo(privilegedClient client.Client) *DropletRepo {
	return &DropletRepo{privilegedClient: privilegedClient}
}

func (r *DropletRepo) FetchDroplet(ctx context.Context, userClient client.Client, dropletGUID string) (DropletRecord, error) {
	buildList := &workloadsv1alpha1.CFBuildList{}
	err := r.privilegedClient.List(ctx, buildList)
	if err != nil { // untested
		return DropletRecord{}, err
	}
	allBuilds := buildList.Items
	matches := filterBuildsByMetadataName(allBuilds, dropletGUID)

	return r.returnDroplet(matches)
}

func (r *DropletRepo) returnDroplet(builds []workloadsv1alpha1.CFBuild) (DropletRecord, error) {
	if len(builds) == 0 {
		return DropletRecord{}, NotFoundError{}
	}
	if len(builds) > 1 { // untested
		return DropletRecord{}, errors.New("duplicate builds exist")
	}

	cfBuild := builds[0]
	stagingStatus := getConditionValue(&cfBuild.Status.Conditions, StagingConditionType)
	succeededStatus := getConditionValue(&cfBuild.Status.Conditions, SucceededConditionType)
	if stagingStatus == metav1.ConditionFalse &&
		succeededStatus == metav1.ConditionTrue {
		return cfBuildToDropletRecord(cfBuild), nil
	}
	return DropletRecord{}, NotFoundError{}
}

func cfBuildToDropletRecord(cfBuild workloadsv1alpha1.CFBuild) DropletRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfBuild.ObjectMeta)
	processTypesMap := make(map[string]string)
	processTypesArrayObject := cfBuild.Status.BuildDropletStatus.ProcessTypes
	for index := range processTypesArrayObject {
		processTypesMap[processTypesArrayObject[index].Type] = processTypesArrayObject[index].Command
	}

	return DropletRecord{
		GUID:      cfBuild.Name,
		State:     "STAGED",
		CreatedAt: formatTimestamp(cfBuild.CreationTimestamp),
		UpdatedAt: updatedAtTime,
		Lifecycle: Lifecycle{
			Type: string(cfBuild.Spec.Lifecycle.Type),
			Data: LifecycleData{
				Buildpacks: []string{},
				Stack:      cfBuild.Spec.Lifecycle.Data.Stack,
			},
		},
		Stack:        cfBuild.Status.BuildDropletStatus.Stack,
		ProcessTypes: processTypesMap,
		AppGUID:      cfBuild.Spec.AppRef.Name,
		PackageGUID:  cfBuild.Spec.PackageRef.Name,
		Labels:       cfBuild.Labels,
		Annotations:  cfBuild.Annotations,
	}
}
