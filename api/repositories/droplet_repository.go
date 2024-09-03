package repositories

import (
	"context"
	"fmt"
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
	CreatedAt       time.Time
	UpdatedAt       *time.Time
	DropletErrorMsg string
	Lifecycle       Lifecycle
	Stack           string
	ProcessTypes    map[string]string
	AppGUID         string
	PackageGUID     string
	Labels          map[string]string
	Annotations     map[string]string
	Image           string
	Ports           []int32
}

func (r DropletRecord) Relationships() map[string]string {
	return map[string]string{
		"app": r.AppGUID,
	}
}

type ListDropletsMessage struct {
	PackageGUIDs []string
}

func (m *ListDropletsMessage) matches(b korifiv1alpha1.CFBuild) bool {
	return tools.EmptyOrContains(m.PackageGUIDs, b.Spec.PackageRef.Name) &&
		meta.IsStatusConditionFalse(b.Status.Conditions, StagingConditionType) &&
		meta.IsStatusConditionTrue(b.Status.Conditions, SucceededConditionType)
}

func (r *DropletRepo) GetDroplet(ctx context.Context, authInfo authorization.Info, dropletGUID string) (DropletRecord, error) {
	build, _, err := r.getBuildAssociatedWithDroplet(ctx, authInfo, dropletGUID)
	if err != nil {
		return DropletRecord{}, err
	}

	return cfBuildToDroplet(build)
}

func (r *DropletRepo) getBuildAssociatedWithDroplet(ctx context.Context, authInfo authorization.Info, dropletGUID string) (*korifiv1alpha1.CFBuild, client.WithWatch, error) {
	// A droplet is a subset of a build
	ns, err := r.namespaceRetriever.NamespaceFor(ctx, dropletGUID, DropletResourceType)
	if err != nil {
		return nil, nil, err
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build user client: %w", err)
	}

	var build korifiv1alpha1.CFBuild
	err = userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: dropletGUID}, &build)
	if err != nil {
		return nil, nil, apierrors.FromK8sError(err, DropletResourceType)
	}
	return &build, userClient, nil
}

func cfBuildToDroplet(cfBuild *korifiv1alpha1.CFBuild) (DropletRecord, error) {
	stagingStatus := getConditionValue(&cfBuild.Status.Conditions, StagingConditionType)
	succeededStatus := getConditionValue(&cfBuild.Status.Conditions, SucceededConditionType)
	if stagingStatus == metav1.ConditionFalse &&
		succeededStatus == metav1.ConditionTrue {
		return cfBuildToDropletRecord(*cfBuild), nil
	}
	return DropletRecord{}, apierrors.NewNotFoundError(nil, DropletResourceType)
}

func cfBuildToDropletRecord(cfBuild korifiv1alpha1.CFBuild) DropletRecord {
	processTypesMap := make(map[string]string)
	processTypesArrayObject := cfBuild.Status.Droplet.ProcessTypes
	for index := range processTypesArrayObject {
		processTypesMap[processTypesArrayObject[index].Type] = processTypesArrayObject[index].Command
	}

	result := DropletRecord{
		GUID:      cfBuild.Name,
		State:     "STAGED",
		CreatedAt: cfBuild.CreationTimestamp.Time,
		UpdatedAt: getLastUpdatedTime(&cfBuild),
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
		Ports:        cfBuild.Status.Droplet.Ports,
	}

	if cfBuild.Spec.Lifecycle.Type == "docker" {
		result.Lifecycle.Data = LifecycleData{}
		result.Image = cfBuild.Status.Droplet.Registry.Image
	}

	return result
}

func (r *DropletRepo) ListDroplets(ctx context.Context, authInfo authorization.Info, message ListDropletsMessage) ([]DropletRecord, error) {
	buildList := &korifiv1alpha1.CFBuildList{}

	namespaces, err := r.namespacePermissions.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []DropletRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	var allBuilds []korifiv1alpha1.CFBuild
	for ns := range namespaces {
		err := userClient.List(ctx, buildList, client.InNamespace(ns))
		if k8serrors.IsForbidden(err) {
			continue
		}
		if err != nil {
			return []DropletRecord{}, apierrors.FromK8sError(err, BuildResourceType)
		}
		allBuilds = append(allBuilds, buildList.Items...)
	}

	filteredBuilds := itx.FromSlice(allBuilds).Filter(message.matches)
	return slices.Collect(it.Map(filteredBuilds, cfBuildToDropletRecord)), nil
}

type UpdateDropletMessage struct {
	GUID          string
	MetadataPatch MetadataPatch
}

func (r *DropletRepo) UpdateDroplet(ctx context.Context, authInfo authorization.Info, message UpdateDropletMessage) (DropletRecord, error) {
	build, userClient, err := r.getBuildAssociatedWithDroplet(ctx, authInfo, message.GUID)
	if err != nil {
		return DropletRecord{}, err
	}

	err = k8s.PatchResource(ctx, userClient, build, func() {
		message.MetadataPatch.Apply(build)
	})
	if err != nil {
		return DropletRecord{}, fmt.Errorf("failed to patch droplet metadata: %w", apierrors.FromK8sError(err, DropletResourceType))
	}

	return cfBuildToDroplet(build)
}
