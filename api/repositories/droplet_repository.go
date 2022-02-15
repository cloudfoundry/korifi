package repositories

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// No kubebuilder RBAC tags required, because Build and Droplet are the same CR

type DropletRepo struct {
	privilegedClient  client.Client
	userClientFactory UserK8sClientFactory
}

func NewDropletRepo(privilegedClient client.Client, userClientFactory UserK8sClientFactory) *DropletRepo {
	return &DropletRepo{
		privilegedClient:  privilegedClient,
		userClientFactory: userClientFactory,
	}
}

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

type ListDropletsMessage struct {
	PackageGUIDs []string
}

func (r *DropletRepo) GetDroplet(ctx context.Context, authInfo authorization.Info, dropletGUID string) (DropletRecord, error) {
	buildList := &workloadsv1alpha1.CFBuildList{}
	err := r.privilegedClient.List(ctx, buildList, client.MatchingFields{"metadata.name": dropletGUID})
	if err != nil { // untested
		return DropletRecord{}, err
	}
	builds := buildList.Items
	if len(builds) == 0 {
		return DropletRecord{}, NewNotFoundError("Droplet", nil)
	}
	if len(builds) > 1 { // untested
		return DropletRecord{}, errors.New("duplicate builds exist")
	}

	foundObj := builds[0]
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return DropletRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var userDroplet workloadsv1alpha1.CFBuild
	err = userClient.Get(ctx, client.ObjectKeyFromObject(&foundObj), &userDroplet)
	if err != nil {
		if k8serrors.IsForbidden(err) {
			return DropletRecord{}, NewForbiddenError("Droplet", err)
		}

		return DropletRecord{}, fmt.Errorf("get droplet failed: %w", err)
	}

	return returnDroplet([]workloadsv1alpha1.CFBuild{userDroplet})
}

func returnDroplet(builds []workloadsv1alpha1.CFBuild) (DropletRecord, error) {
	cfBuild := builds[0]
	stagingStatus := getConditionValue(&cfBuild.Status.Conditions, StagingConditionType)
	succeededStatus := getConditionValue(&cfBuild.Status.Conditions, SucceededConditionType)
	if stagingStatus == metav1.ConditionFalse &&
		succeededStatus == metav1.ConditionTrue {
		return cfBuildToDropletRecord(cfBuild), nil
	}
	return DropletRecord{}, NewNotFoundError("Droplet", nil)
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

func (r *DropletRepo) ListDroplets(ctx context.Context, authInfo authorization.Info, message ListDropletsMessage) ([]DropletRecord, error) {
	buildList := &workloadsv1alpha1.CFBuildList{}
	err := r.privilegedClient.List(ctx, buildList)
	if err != nil { // untested
		return []DropletRecord{}, err
	}
	allBuilds := buildList.Items
	matches := applyDropletFilters(allBuilds, message)

	return returnDropletList(matches), nil
}

func returnDropletList(droplets []workloadsv1alpha1.CFBuild) []DropletRecord {
	dropletRecords := make([]DropletRecord, 0, len(droplets))

	for _, currentBuild := range droplets {
		dropletRecords = append(dropletRecords, cfBuildToDropletRecord(currentBuild))
	}
	return dropletRecords
}

func applyDropletFilters(builds []workloadsv1alpha1.CFBuild, message ListDropletsMessage) []workloadsv1alpha1.CFBuild {
	var filtered []workloadsv1alpha1.CFBuild
	for i, build := range builds {

		stagingStatus := getConditionValue(&build.Status.Conditions, StagingConditionType)
		succeededStatus := getConditionValue(&build.Status.Conditions, SucceededConditionType)
		if stagingStatus != metav1.ConditionFalse ||
			succeededStatus != metav1.ConditionTrue {
			continue
		}

		if len(message.PackageGUIDs) > 0 {
			foundMatch := false
			for _, packageGUID := range message.PackageGUIDs {
				if build.Spec.PackageRef.Name == packageGUID {
					foundMatch = true
					break
				}
			}
			if !foundMatch {
				continue
			}
		}

		filtered = append(filtered, builds[i])
	}
	return filtered
}
