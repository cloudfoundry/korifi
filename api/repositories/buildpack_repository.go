package repositories

import (
	"context"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
)

const (
	BuildpackResourceType = "Buildpack"
)

type BuildpackRepository struct {
	builderName       string
	userClientFactory authorization.UserK8sClientFactory
	rootNamespace     string
}

type BuildpackRecord struct {
	Name      string
	Position  int
	Stack     string
	Version   string
	CreatedAt time.Time
	UpdatedAt *time.Time
}

func NewBuildpackRepository(builderName string, userClientFactory authorization.UserK8sClientFactory, rootNamespace string) *BuildpackRepository {
	return &BuildpackRepository{
		builderName:       builderName,
		userClientFactory: userClientFactory,
		rootNamespace:     rootNamespace,
	}
}

func (r *BuildpackRepository) ListBuildpacks(ctx context.Context, authInfo authorization.Info) ([]BuildpackRecord, error) {
	var builderInfo v1alpha1.BuilderInfo

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	err = userClient.Get(
		ctx,
		types.NamespacedName{
			Namespace: r.rootNamespace,
			Name:      r.builderName,
		},
		&builderInfo,
	)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, apierrors.NewResourceNotReadyError(fmt.Errorf("BuilderInfo %q not found in namespace %q", r.builderName, r.rootNamespace))
		}

		return nil, apierrors.FromK8sError(err, BuildpackResourceType)
	}

	if !meta.IsStatusConditionTrue(builderInfo.Status.Conditions, StatusConditionReady) {
		var conditionNotReadyMessage string

		readyCondition := meta.FindStatusCondition(builderInfo.Status.Conditions, StatusConditionReady)
		if readyCondition != nil {
			conditionNotReadyMessage = readyCondition.Message
		}

		if conditionNotReadyMessage == "" {
			conditionNotReadyMessage = "resource not reconciled"
		}

		return nil, apierrors.NewResourceNotReadyError(fmt.Errorf("BuilderInfo %q not ready: %s", r.builderName, conditionNotReadyMessage))
	}

	return builderInfoToBuildpackRecords(builderInfo), nil
}

func builderInfoToBuildpackRecords(info v1alpha1.BuilderInfo) []BuildpackRecord {
	buildpackRecords := make([]BuildpackRecord, 0, len(info.Status.Buildpacks))

	for i := range info.Status.Buildpacks {
		b := info.Status.Buildpacks[i]
		currentRecord := BuildpackRecord{
			Name:      b.Name,
			Version:   b.Version,
			Position:  i + 1,
			Stack:     b.Stack,
			CreatedAt: b.CreationTimestamp.Time,
			UpdatedAt: &b.UpdatedTimestamp.Time,
		}
		buildpackRecords = append(buildpackRecords, currentRecord)
	}

	return buildpackRecords
}
