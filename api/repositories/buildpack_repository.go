package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const (
	BuildpackResourceType = "Buildpack"
)

type BuildpackRepository struct {
	buildReconciler   string
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

func NewBuildpackRepository(buildReconciler string, userClientFactory authorization.UserK8sClientFactory, rootNamespace string) *BuildpackRepository {
	return &BuildpackRepository{
		buildReconciler:   buildReconciler,
		userClientFactory: userClientFactory,
		rootNamespace:     rootNamespace,
	}
}

func (r *BuildpackRepository) ListBuildpacks(ctx context.Context, authInfo authorization.Info) ([]BuildpackRecord, error) {
	var buildReconcilerInfo v1alpha1.BuildReconcilerInfo

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	err = userClient.Get(
		ctx,
		types.NamespacedName{
			Namespace: r.rootNamespace,
			Name:      r.buildReconciler,
		},
		&buildReconcilerInfo,
	)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("no BuildReconcilerInfo %q resource found in %q namespace", r.buildReconciler, r.rootNamespace)
		}
		return nil, apierrors.FromK8sError(err, BuildpackResourceType)
	}

	return buildReconcilerInfoToBuildpackRecords(buildReconcilerInfo), nil
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
