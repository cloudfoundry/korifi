package repositories

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/pivotal/kpack/pkg/logs"
	"k8s.io/apimachinery/pkg/labels"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

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

	BuildResourceType = "Build"
)

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

type LogRecord struct {
	Message   string
	Timestamp int64
}

type BuildRepo struct {
	namespaceRetriever NamespaceRetriever
	userClientFactory  authorization.UserK8sClientFactory
}

func NewBuildRepo(namespaceRetriever NamespaceRetriever, userClientFactory authorization.UserK8sClientFactory) *BuildRepo {
	return &BuildRepo{
		namespaceRetriever: namespaceRetriever,
		userClientFactory:  userClientFactory,
	}
}

func (b *BuildRepo) GetBuild(ctx context.Context, authInfo authorization.Info, buildGUID string) (BuildRecord, error) {
	ns, err := b.namespaceRetriever.NamespaceFor(ctx, buildGUID, BuildResourceType)
	if err != nil {
		return BuildRecord{}, err
	}

	userClient, err := b.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return BuildRecord{}, fmt.Errorf("get-build failed to build user client: %w", err)
	}

	build := v1alpha1.CFBuild{}
	if err := userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: buildGUID}, &build); err != nil {
		return BuildRecord{}, fmt.Errorf("failed to get build: %w", apierrors.FromK8sError(err, BuildResourceType))
	}

	return cfBuildToBuildRecord(build), nil
}

func (b *BuildRepo) GetLatestBuildByAppGUID(ctx context.Context, authInfo authorization.Info, spaceGUID string, appGUID string) (BuildRecord, error) {
	userClient, err := b.userClientFactory.BuildClient(authInfo)
	if err != nil { // Untested
		return BuildRecord{}, apierrors.NewUnknownError(err)
	}
	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{
		v1alpha1.CFAppGUIDLabelKey: appGUID,
	})
	if err != nil { // Untested
		return BuildRecord{}, apierrors.NewUnknownError(err)
	}

	listOpts := &client.ListOptions{Namespace: spaceGUID, LabelSelector: labelSelector}
	buildList := &v1alpha1.CFBuildList{}

	err = userClient.List(ctx, buildList, listOpts)
	if err != nil {
		return BuildRecord{}, apierrors.FromK8sError(err, BuildResourceType)
	}

	if len(buildList.Items) == 0 {
		return BuildRecord{}, apierrors.NewNotFoundError(fmt.Errorf("builds for app %q in space %q not found", appGUID, spaceGUID), BuildResourceType)
	}

	sortedBuilds := orderBuilds(buildList.Items)

	return cfBuildToBuildRecord(sortedBuilds[0]), nil
}

func orderBuilds(builds []v1alpha1.CFBuild) []v1alpha1.CFBuild {
	sort.Slice(builds, func(i, j int) bool {
		return !builds[i].CreationTimestamp.Before(&builds[j].CreationTimestamp)
	})
	return builds
}

func (b *BuildRepo) GetBuildLogs(ctx context.Context, authInfo authorization.Info, spaceGUID string, buildGUID string) ([]LogRecord, error) {
	userClient, err := b.userClientFactory.BuildK8sClient(authInfo)
	if err != nil { // Untested
		return []LogRecord{}, apierrors.NewUnknownError(err)
	}
	logWriter := new(strings.Builder)
	err = logs.NewBuildLogsClient(userClient).GetImageLogs(ctx, logWriter, buildGUID, spaceGUID)
	if err != nil {
		return nil, err
	}
	buildLogs := strings.Split(logWriter.String(), "\n")
	toReturn := make([]LogRecord, 0, len(buildLogs))
	for _, log := range buildLogs {
		toReturn = append(toReturn, LogRecord{
			Message: log,
			// TODO: Parse timestamps once we sort out kpack GetImageLogs
		})
	}
	return toReturn, nil
}

func cfBuildToBuildRecord(cfBuild v1alpha1.CFBuild) BuildRecord {
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
		switch succeededStatus {
		case metav1.ConditionTrue:
			toReturn.State = BuildStateStaged
			toReturn.DropletGUID = cfBuild.Name
		case metav1.ConditionFalse:
			toReturn.State = BuildStateFailed
			conditionStatus := meta.FindStatusCondition(cfBuild.Status.Conditions, SucceededConditionType)
			toReturn.StagingErrorMsg = fmt.Sprintf("%v: %v", conditionStatus.Reason, conditionStatus.Message)
		}
	}

	return toReturn
}

func (b *BuildRepo) CreateBuild(ctx context.Context, authInfo authorization.Info, message CreateBuildMessage) (BuildRecord, error) {
	cfBuild := message.toCFBuild()
	userClient, err := b.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return BuildRecord{}, fmt.Errorf("failed to build user k8s client: %w", err)
	}

	if err := userClient.Create(ctx, &cfBuild); err != nil {
		return BuildRecord{}, apierrors.FromK8sError(err, BuildResourceType)
	}
	return cfBuildToBuildRecord(cfBuild), nil
}

type CreateBuildMessage struct {
	AppGUID         string
	OwnerRef        metav1.OwnerReference
	PackageGUID     string
	SpaceGUID       string
	StagingMemoryMB int
	StagingDiskMB   int
	Lifecycle       Lifecycle
	Labels          map[string]string
	Annotations     map[string]string
}

func (m CreateBuildMessage) toCFBuild() v1alpha1.CFBuild {
	guid := uuid.NewString()
	return v1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:        guid,
			Namespace:   m.SpaceGUID,
			Labels:      m.Labels,
			Annotations: m.Annotations,
			OwnerReferences: []metav1.OwnerReference{
				m.OwnerRef,
			},
		},
		Spec: v1alpha1.CFBuildSpec{
			PackageRef: corev1.LocalObjectReference{
				Name: m.PackageGUID,
			},
			AppRef: corev1.LocalObjectReference{
				Name: m.AppGUID,
			},
			StagingMemoryMB: m.StagingMemoryMB,
			StagingDiskMB:   m.StagingDiskMB,
			Lifecycle: v1alpha1.Lifecycle{
				Type: v1alpha1.LifecycleType(m.Lifecycle.Type),
				Data: v1alpha1.LifecycleData{
					Buildpacks: m.Lifecycle.Data.Buildpacks,
					Stack:      m.Lifecycle.Data.Stack,
				},
			},
		},
	}
}
