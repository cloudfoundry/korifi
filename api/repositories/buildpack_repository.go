package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	BuildpackResourceType = "Buildpack"
)

type BuildpackRepository struct {
	userClientFactory authorization.UserK8sClientFactory
	rootNamespace     string
}

type BuildpackRecord struct {
	Name      string
	Position  int
	Stack     string
	Version   string
	CreatedAt string
	UpdatedAt string
}

type ListBuildpacksMessage struct {
	OrderBy []string
}

func NewBuildpackRepository(userClientFactory authorization.UserK8sClientFactory, rootNamespace string) *BuildpackRepository {
	return &BuildpackRepository{
		userClientFactory: userClientFactory,
		rootNamespace:     rootNamespace,
	}
}

func (r *BuildpackRepository) ListBuildpacks(ctx context.Context, authInfo authorization.Info) ([]BuildpackRecord, error) {
	var buildReconcilerInfos v1alpha1.BuildReconcilerInfoList

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	err = userClient.List(ctx, &buildReconcilerInfos, client.InNamespace(r.rootNamespace))
	if err != nil {
		return nil, apierrors.FromK8sError(err, BuildpackResourceType)
	}

	switch len(buildReconcilerInfos.Items) {
	case 0:
		return nil, fmt.Errorf("no BuildReconcilerInfo resource found in %q namespace", r.rootNamespace)
	case 1:
		buildReconcilerInfo := buildReconcilerInfos.Items[0]
		return buildReconcilerInfoToBuildpackRecords(buildReconcilerInfo), nil
	default:
		return nil, fmt.Errorf("more than 1 BuildReconcilerInfo resource found in %q namespace", r.rootNamespace)
	}
}

func buildReconcilerInfoToBuildpackRecords(info v1alpha1.BuildReconcilerInfo) []BuildpackRecord {
	buildpackRecords := make([]BuildpackRecord, 0, len(info.Status.Buildpacks))
	for i, b := range info.Status.Buildpacks {
		currentRecord := BuildpackRecord{
			Name:      b.Name,
			Version:   b.Version,
			Position:  i + 1,
			Stack:     b.Stack,
			CreatedAt: b.CreationTimestamp.UTC().Format(TimestampFormat),
			UpdatedAt: b.UpdatedTimestamp.UTC().Format(TimestampFormat),
		}
		buildpackRecords = append(buildpackRecords, currentRecord)
	}
	return buildpackRecords
}
