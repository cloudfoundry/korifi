package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/controllers/apis/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// No kubebuilder RBAC tags required, because Build and Droplet are the same CR

const (
	DropletResourceType = "Droplet"
)

type DropletRepo struct {
	userClientFactory    authorization.UserK8sClientFactory
	namespaceRetriever   NamespaceRetriever
	namespacePermissions *authorization.NamespacePermissions
}

func NewDropletRepo(
	userClientFactory authorization.UserK8sClientFactory,
	namespaceRetriever NamespaceRetriever,
	namespacePermissions *authorization.NamespacePermissions,
) *DropletRepo {
	return &DropletRepo{
		userClientFactory:    userClientFactory,
		namespaceRetriever:   namespaceRetriever,
		namespacePermissions: namespacePermissions,
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
	// A droplet is a subset of a build
	ns, err := r.namespaceRetriever.NamespaceFor(ctx, dropletGUID, DropletResourceType)
	if err != nil {
		return DropletRecord{}, err
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return DropletRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var userDroplet v1alpha1.CFBuild
	err = userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: dropletGUID}, &userDroplet)
	if err != nil {
		return DropletRecord{}, apierrors.FromK8sError(err, DropletResourceType)
	}

	return returnDroplet(userDroplet)
}

func returnDroplet(cfBuild v1alpha1.CFBuild) (DropletRecord, error) {
	stagingStatus := getConditionValue(&cfBuild.Status.Conditions, StagingConditionType)
	succeededStatus := getConditionValue(&cfBuild.Status.Conditions, SucceededConditionType)
	if stagingStatus == metav1.ConditionFalse &&
		succeededStatus == metav1.ConditionTrue {
		return cfBuildToDropletRecord(cfBuild), nil
	}
	return DropletRecord{}, apierrors.NewNotFoundError(nil, DropletResourceType)
}

func cfBuildToDropletRecord(cfBuild v1alpha1.CFBuild) DropletRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfBuild.ObjectMeta)
	processTypesMap := make(map[string]string)
	processTypesArrayObject := cfBuild.Status.Droplet.ProcessTypes
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
		Stack:        cfBuild.Status.Droplet.Stack,
		ProcessTypes: processTypesMap,
		AppGUID:      cfBuild.Spec.AppRef.Name,
		PackageGUID:  cfBuild.Spec.PackageRef.Name,
		Labels:       cfBuild.Labels,
		Annotations:  cfBuild.Annotations,
	}
}

func (r *DropletRepo) ListDroplets(ctx context.Context, authInfo authorization.Info, message ListDropletsMessage) ([]DropletRecord, error) {
	buildList := &v1alpha1.CFBuildList{}

	namespaces, err := r.namespacePermissions.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []DropletRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var allBuilds []v1alpha1.CFBuild
	for ns := range namespaces {
		err := userClient.List(ctx, buildList, client.InNamespace(ns))
		if err != nil {
			return []DropletRecord{}, apierrors.FromK8sError(err, BuildResourceType)
		}
		allBuilds = append(allBuilds, buildList.Items...)
	}
	matches := applyDropletFilters(allBuilds, message)

	return returnDropletList(matches), nil
}

func returnDropletList(droplets []v1alpha1.CFBuild) []DropletRecord {
	dropletRecords := make([]DropletRecord, 0, len(droplets))

	for _, currentBuild := range droplets {
		dropletRecords = append(dropletRecords, cfBuildToDropletRecord(currentBuild))
	}
	return dropletRecords
}

func applyDropletFilters(builds []v1alpha1.CFBuild, message ListDropletsMessage) []v1alpha1.CFBuild {
	var filtered []v1alpha1.CFBuild
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
