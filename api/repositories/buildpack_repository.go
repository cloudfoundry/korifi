package repositories

import (
	"context"
	"fmt"
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories/compare"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/BooleanCat/go-functional/v2/it"
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
	sorter            BuildpackSorter
}

type BuildpackRecord struct {
	Name      string
	Position  int
	Stack     string
	Version   string
	CreatedAt time.Time
	UpdatedAt *time.Time
}

//counterfeiter:generate -o fake -fake-name BuildpackSorter . BuildpackSorter
type BuildpackSorter interface {
	Sort(records []BuildpackRecord, order string) []BuildpackRecord
}

type buildpackSorter struct {
	sorter *compare.Sorter[BuildpackRecord]
}

func NewBuildpackSorter() *buildpackSorter {
	return &buildpackSorter{
		sorter: compare.NewSorter(BuildpackComparator),
	}
}

func (s *buildpackSorter) Sort(records []BuildpackRecord, order string) []BuildpackRecord {
	return s.sorter.Sort(records, order)
}

func BuildpackComparator(fieldName string) func(BuildpackRecord, BuildpackRecord) int {
	return func(b1, b2 BuildpackRecord) int {
		switch fieldName {
		case "created_at":
			return tools.CompareTimePtr(&b1.CreatedAt, &b2.CreatedAt)
		case "-created_at":
			return tools.CompareTimePtr(&b2.CreatedAt, &b1.CreatedAt)
		case "updated_at":
			return tools.CompareTimePtr(b1.UpdatedAt, b2.UpdatedAt)
		case "-updated_at":
			return tools.CompareTimePtr(b2.UpdatedAt, b1.UpdatedAt)
		case "position":
			return b1.Position - b2.Position
		case "-position":
			return b2.Position - b1.Position
		}
		return 0
	}
}

type ListBuildpacksMessage struct {
	OrderBy string
}

func NewBuildpackRepository(
	builderName string,
	userClientFactory authorization.UserK8sClientFactory,
	rootNamespace string,
	sorter BuildpackSorter,
) *BuildpackRepository {
	return &BuildpackRepository{
		builderName:       builderName,
		userClientFactory: userClientFactory,
		rootNamespace:     rootNamespace,
		sorter:            sorter,
	}
}

func (r *BuildpackRepository) ListBuildpacks(ctx context.Context, authInfo authorization.Info, message ListBuildpacksMessage) ([]BuildpackRecord, error) {
	var builderInfo korifiv1alpha1.BuilderInfo

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

	if !meta.IsStatusConditionTrue(builderInfo.Status.Conditions, korifiv1alpha1.StatusConditionReady) {
		var conditionNotReadyMessage string

		readyCondition := meta.FindStatusCondition(builderInfo.Status.Conditions, korifiv1alpha1.StatusConditionReady)
		if readyCondition != nil {
			conditionNotReadyMessage = readyCondition.Message
		}

		if conditionNotReadyMessage == "" {
			conditionNotReadyMessage = "resource not reconciled"
		}

		return nil, apierrors.NewResourceNotReadyError(fmt.Errorf("BuilderInfo %q not ready: %s", r.builderName, conditionNotReadyMessage))
	}

	return r.sorter.Sort(builderInfoToBuildpackRecords(builderInfo), message.OrderBy), nil
}

func builderInfoToBuildpackRecords(info korifiv1alpha1.BuilderInfo) []BuildpackRecord {
	return slices.Collect(it.Right(it.Map2(slices.All(info.Status.Buildpacks), func(i int, b korifiv1alpha1.BuilderInfoStatusBuildpack) (int, BuildpackRecord) {
		return i, BuildpackRecord{
			Name:      b.Name,
			Version:   b.Version,
			Position:  i + 1,
			Stack:     b.Stack,
			CreatedAt: b.CreationTimestamp.Time,
			UpdatedAt: &b.UpdatedTimestamp.Time,
		}
	})))
}
