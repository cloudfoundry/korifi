package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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

//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfbuilds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=workloads.cloudfoundry.org,resources=cfbuilds/status,verbs=get

type BuildRepo struct {
	privilegedClient   client.Client
	namespaceRetriever NamespaceRetriever
	userClientFactory  UserK8sClientFactory
}

func NewBuildRepo(privilegedClient client.Client, namespaceRetriever NamespaceRetriever, userClientFactory UserK8sClientFactory) *BuildRepo {
	return &BuildRepo{
		privilegedClient:   privilegedClient,
		namespaceRetriever: namespaceRetriever,
		userClientFactory:  userClientFactory,
	}
}

// nolint: dupl
func (b *BuildRepo) GetBuild(ctx context.Context, authInfo authorization.Info, buildGUID string) (BuildRecord, error) {
	ns, err := b.namespaceRetriever.NamespaceFor(ctx, buildGUID, BuildResourceType)
	if err != nil {
		return BuildRecord{}, err
	}

	userClient, err := b.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return BuildRecord{}, fmt.Errorf("get-build failed to build user client: %w", err)
	}

	build := workloadsv1alpha1.CFBuild{}
	if err := userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: buildGUID}, &build); err != nil {
		if k8serrors.IsForbidden(err) {
			return BuildRecord{}, NewForbiddenError(BuildResourceType, err)
		}
		return BuildRecord{}, fmt.Errorf("failed to get build: %w", err)
	}

	return cfBuildToBuildRecord(build), nil
}

func cfBuildToBuildRecord(cfBuild workloadsv1alpha1.CFBuild) BuildRecord {
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
		if k8serrors.IsForbidden(err) {
			return BuildRecord{}, NewForbiddenError(BuildResourceType, err)
		}
		return BuildRecord{}, err
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

func (m CreateBuildMessage) toCFBuild() workloadsv1alpha1.CFBuild {
	guid := uuid.NewString()
	return workloadsv1alpha1.CFBuild{
		ObjectMeta: metav1.ObjectMeta{
			Name:        guid,
			Namespace:   m.SpaceGUID,
			Labels:      m.Labels,
			Annotations: m.Annotations,
			OwnerReferences: []metav1.OwnerReference{
				m.OwnerRef,
			},
		},
		Spec: workloadsv1alpha1.CFBuildSpec{
			PackageRef: corev1.LocalObjectReference{
				Name: m.PackageGUID,
			},
			AppRef: corev1.LocalObjectReference{
				Name: m.AppGUID,
			},
			StagingMemoryMB: m.StagingMemoryMB,
			StagingDiskMB:   m.StagingDiskMB,
			Lifecycle: workloadsv1alpha1.Lifecycle{
				Type: workloadsv1alpha1.LifecycleType(m.Lifecycle.Type),
				Data: workloadsv1alpha1.LifecycleData{
					Buildpacks: m.Lifecycle.Data.Buildpacks,
					Stack:      m.Lifecycle.Data.Stack,
				},
			},
		},
	}
}
