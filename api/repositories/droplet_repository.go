package repositories

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/BooleanCat/go-functional/v2/it"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// No kubebuilder RBAC tags required, because Build and Droplet are the same CR

const (
	DropletResourceType = "Droplet"
)

type DropletRepo struct {
	klient Klient
}

func NewDropletRepo(
	klient Klient,
) *DropletRepo {
	return &DropletRepo{
		klient: klient,
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
	GUIDs        []string
	PackageGUIDs []string
	AppGUIDs     []string
	SpaceGUIDs   []string
	OrderBy      string
	Pagination   Pagination
}

func (m *ListDropletsMessage) toListOptions() []ListOption {
	return []ListOption{
		WithLabelIn(korifiv1alpha1.CFPackageGUIDLabelKey, m.PackageGUIDs),
		WithLabelIn(korifiv1alpha1.CFAppGUIDLabelKey, m.AppGUIDs),
		WithLabelIn(korifiv1alpha1.SpaceGUIDLabelKey, m.SpaceGUIDs),
		WithLabelIn(korifiv1alpha1.CFDropletGUIDLabelKey, m.GUIDs),
		WithLabel(korifiv1alpha1.CFBuildStateLabelKey, korifiv1alpha1.BuildStateStaged),
		WithOrdering(m.OrderBy),
		WithPaging(m.Pagination),
	}
}

func (r *DropletRepo) GetDroplet(ctx context.Context, authInfo authorization.Info, dropletGUID string) (DropletRecord, error) {
	build, err := r.getBuildAssociatedWithDroplet(ctx, authInfo, dropletGUID)
	if err != nil {
		return DropletRecord{}, err
	}

	return cfBuildToDroplet(build)
}

func (r *DropletRepo) getBuildAssociatedWithDroplet(ctx context.Context, authInfo authorization.Info, dropletGUID string) (*korifiv1alpha1.CFBuild, error) {
	build := &korifiv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name: dropletGUID,
		},
	}
	err := r.klient.Get(ctx, build)
	if err != nil {
		return nil, apierrors.FromK8sError(err, DropletResourceType)
	}
	return build, nil
}

func cfBuildToDroplet(cfBuild *korifiv1alpha1.CFBuild) (DropletRecord, error) {
	if cfBuild.Status.State == korifiv1alpha1.BuildStateStaged {
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

func (r *DropletRepo) ListDroplets(ctx context.Context, authInfo authorization.Info, message ListDropletsMessage) (ListResult[DropletRecord], error) {
	buildList := &korifiv1alpha1.CFBuildList{}
	pageInfo, err := r.klient.List(ctx, buildList, message.toListOptions()...)
	if err != nil {
		return ListResult[DropletRecord]{}, apierrors.FromK8sError(err, BuildResourceType)
	}

	return ListResult[DropletRecord]{
		Records:  slices.Collect(it.Map(slices.Values(buildList.Items), cfBuildToDropletRecord)),
		PageInfo: pageInfo,
	}, nil
}

type UpdateDropletMessage struct {
	GUID          string
	MetadataPatch MetadataPatch
}

func (r *DropletRepo) UpdateDroplet(ctx context.Context, authInfo authorization.Info, message UpdateDropletMessage) (DropletRecord, error) {
	build, err := r.getBuildAssociatedWithDroplet(ctx, authInfo, message.GUID)
	if err != nil {
		return DropletRecord{}, err
	}

	err = r.klient.Patch(ctx, build, func() error {
		message.MetadataPatch.Apply(build)

		return nil
	})
	if err != nil {
		return DropletRecord{}, fmt.Errorf("failed to patch droplet metadata: %w", apierrors.FromK8sError(err, DropletResourceType))
	}

	return cfBuildToDroplet(build)
}
