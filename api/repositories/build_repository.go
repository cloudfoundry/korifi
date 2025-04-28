package repositories

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories/compare"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	BuildStateStaging = "STAGING"
	BuildStateStaged  = "STAGED"
	BuildStateFailed  = "FAILED"

	StagingConditionType   = "Staging"
	SucceededConditionType = "Succeeded"

	BuildResourceType = "Build"
)

type BuildRecord struct {
	GUID            string
	SpaceGUID       string
	State           string
	CreatedAt       time.Time
	UpdatedAt       *time.Time
	StagingErrorMsg string
	StagingMemoryMB int
	StagingDiskMB   int
	Lifecycle       Lifecycle
	PackageGUID     string
	DropletGUID     string
	AppGUID         string
	Labels          map[string]string
	Annotations     map[string]string
	ImageRef        string
}

func (r BuildRecord) Relationships() map[string]string {
	return map[string]string{
		"app": r.AppGUID,
	}
}

type BuildRepo struct {
	klient Klient
	sorter BuildSorter
}

func NewBuildRepo(
	klient Klient,
	sorter BuildSorter,
) *BuildRepo {
	return &BuildRepo{
		klient: klient,
		sorter: sorter,
	}
}

func (b *BuildRepo) GetBuild(ctx context.Context, authInfo authorization.Info, buildGUID string) (BuildRecord, error) {
	build := korifiv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name: buildGUID,
		},
	}
	if err := b.klient.Get(ctx, &build); err != nil {
		return BuildRecord{}, fmt.Errorf("failed to get build: %w", apierrors.FromK8sError(err, BuildResourceType))
	}

	return b.cfBuildToBuildRecord(build), nil
}

func (b *BuildRepo) GetLatestBuildByAppGUID(ctx context.Context, authInfo authorization.Info, spaceGUID string, appGUID string) (BuildRecord, error) {
	buildList := &korifiv1alpha1.CFBuildList{}
	err := b.klient.List(ctx, buildList, InNamespace(spaceGUID), WithLabel(korifiv1alpha1.CFAppGUIDLabelKey, appGUID))
	if err != nil {
		return BuildRecord{}, apierrors.FromK8sError(err, BuildResourceType)
	}

	if len(buildList.Items) == 0 {
		return BuildRecord{}, apierrors.NewNotFoundError(fmt.Errorf("builds for app %q in space %q not found", appGUID, spaceGUID), BuildResourceType)
	}

	return b.cfBuildToBuildRecord(sortByAge(buildList.Items)[0]), nil
}

func sortByAge(builds []korifiv1alpha1.CFBuild) []korifiv1alpha1.CFBuild {
	sort.Slice(builds, func(i, j int) bool {
		return !builds[i].CreationTimestamp.Before(&builds[j].CreationTimestamp)
	})
	return builds
}

func (b *BuildRepo) cfBuildToBuildRecord(cfBuild korifiv1alpha1.CFBuild) BuildRecord {
	toReturn := BuildRecord{
		GUID:            cfBuild.Name,
		SpaceGUID:       cfBuild.Namespace,
		State:           BuildStateStaging,
		CreatedAt:       cfBuild.CreationTimestamp.Time,
		UpdatedAt:       getLastUpdatedTime(&cfBuild),
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

	if cfBuild.Spec.Lifecycle.Type == "docker" {
		toReturn.Lifecycle.Data = LifecycleData{}
	}

	if cfBuild.Spec.Lifecycle.Data.Buildpacks != nil {
		toReturn.Lifecycle.Data.Buildpacks = cfBuild.Spec.Lifecycle.Data.Buildpacks
	}

	stagingStatus := getConditionValue(&cfBuild.Status.Conditions, StagingConditionType)
	succeededStatus := getConditionValue(&cfBuild.Status.Conditions, SucceededConditionType)
	// TODO: Consider moving this logic to CRDs repo in case Status Conditions change later?
	if stagingStatus == metav1.ConditionFalse {
		switch succeededStatus {
		case metav1.ConditionTrue:
			toReturn.State = BuildStateStaged
			toReturn.DropletGUID = cfBuild.Name
		case metav1.ConditionFalse:
			toReturn.State = BuildStateFailed
			conditionStatus := meta.FindStatusCondition(cfBuild.Status.Conditions, SucceededConditionType)
			toReturn.StagingErrorMsg = conditionStatus.Message
		}
	}

	return toReturn
}

func (b *BuildRepo) CreateBuild(ctx context.Context, authInfo authorization.Info, message CreateBuildMessage) (BuildRecord, error) {
	cfBuild := message.toCFBuild()
	if err := b.klient.Create(ctx, &cfBuild); err != nil {
		return BuildRecord{}, apierrors.FromK8sError(err, BuildResourceType)
	}

	return b.cfBuildToBuildRecord(cfBuild), nil
}

func (b *BuildRepo) ListBuilds(ctx context.Context, authInfo authorization.Info, message ListBuildsMessage) ([]BuildRecord, error) {
	buildList := &korifiv1alpha1.CFBuildList{}
	err := b.klient.List(ctx, buildList)
	if err != nil {
		return []BuildRecord{}, fmt.Errorf("failed to list builds: %w", apierrors.FromK8sError(err, BuildResourceType))
	}
	filteredBuilds := itx.FromSlice(buildList.Items).Filter(message.matches)

	return b.sorter.Sort(slices.Collect(it.Map(filteredBuilds, b.cfBuildToBuildRecord)), message.OrderBy), nil
}

type CreateBuildMessage struct {
	AppGUID         string
	PackageGUID     string
	SpaceGUID       string
	StagingMemoryMB int
	StagingDiskMB   int
	Lifecycle       Lifecycle
	Labels          map[string]string
	Annotations     map[string]string
}

//counterfeiter:generate -o fake -fake-name BuildSorter . BuildSorter
type BuildSorter interface {
	Sort(records []BuildRecord, order string) []BuildRecord
}

type buildSorter struct {
	sorter *compare.Sorter[BuildRecord]
}

func NewBuildSorter() *buildSorter {
	return &buildSorter{
		sorter: compare.NewSorter(BuildComparator),
	}
}

func (s *buildSorter) Sort(records []BuildRecord, order string) []BuildRecord {
	return s.sorter.Sort(records, order)
}

func BuildComparator(fieldName string) func(BuildRecord, BuildRecord) int {
	return func(d1, d2 BuildRecord) int {
		switch fieldName {
		case "created_at":
			return tools.CompareTimePtr(&d1.CreatedAt, &d2.CreatedAt)
		case "-created_at":
			return tools.CompareTimePtr(&d2.CreatedAt, &d1.CreatedAt)
		case "updated_at":
			return tools.CompareTimePtr(d1.UpdatedAt, d2.UpdatedAt)
		case "-updated_at":
			return tools.CompareTimePtr(d2.UpdatedAt, d1.UpdatedAt)
		}
		return 0
	}
}

type ListBuildsMessage struct {
	PackageGUIDs []string
	AppGUIDs     []string
	States       []string
	OrderBy      string
}

func (m *ListBuildsMessage) matches(b korifiv1alpha1.CFBuild) bool {
	return tools.EmptyOrContains(m.PackageGUIDs, b.Spec.PackageRef.Name) &&

		tools.EmptyOrContains(m.AppGUIDs, b.Spec.AppRef.Name) &&
		m.matchesState(b)
}

func (m *ListBuildsMessage) matchesState(b korifiv1alpha1.CFBuild) bool {
	if len(m.States) == 0 {
		return true
	}

	if slices.Contains(m.States, BuildStateStaged) && meta.IsStatusConditionTrue(b.Status.Conditions, korifiv1alpha1.SucceededConditionType) {
		return true
	}

	if slices.Contains(m.States, BuildStateStaging) && meta.IsStatusConditionTrue(b.Status.Conditions, korifiv1alpha1.StagingConditionType) {
		return true
	}

	if slices.Contains(m.States, BuildStateFailed) && meta.IsStatusConditionFalse(b.Status.Conditions, korifiv1alpha1.SucceededConditionType) {
		return true
	}

	return false
}

func (m CreateBuildMessage) toCFBuild() korifiv1alpha1.CFBuild {
	return korifiv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:        uuid.NewString(),
			Namespace:   m.SpaceGUID,
			Labels:      m.Labels,
			Annotations: m.Annotations,
		},
		Spec: korifiv1alpha1.CFBuildSpec{
			PackageRef: corev1.LocalObjectReference{
				Name: m.PackageGUID,
			},
			AppRef: corev1.LocalObjectReference{
				Name: m.AppGUID,
			},
			StagingMemoryMB: m.StagingMemoryMB,
			StagingDiskMB:   m.StagingDiskMB,
			Lifecycle: korifiv1alpha1.Lifecycle{
				Type: korifiv1alpha1.LifecycleType(m.Lifecycle.Type),
				Data: korifiv1alpha1.LifecycleData{
					Buildpacks: m.Lifecycle.Data.Buildpacks,
					Stack:      m.Lifecycle.Data.Stack,
				},
			},
		},
	}
}
