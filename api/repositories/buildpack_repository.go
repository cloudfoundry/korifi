package repositories

import (
	"context"

	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	"k8s.io/apimachinery/pkg/types"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BuildpackRecord struct {
	Name      string
	Position  int
	Stack     string
	Version   string
	CreatedAt string
	UpdatedAt string
}

type BuildpackRepository struct {
	privilegedClient client.Client
}

func NewBuildpackRepository(privilegedClient client.Client) *BuildpackRepository {
	return &BuildpackRepository{privilegedClient: privilegedClient}
}

func (r *BuildpackRepository) GetBuildpacksForBuilder(ctx context.Context, authInfo authorization.Info, builderName string) ([]BuildpackRecord, error) {
	clusterBuilder := &buildv1alpha2.ClusterBuilder{}
	err := r.privilegedClient.Get(ctx, types.NamespacedName{Name: builderName}, clusterBuilder)
	if err != nil {
		return []BuildpackRecord{}, err
	}

	return clusterBuilderToBuildpackRecords(clusterBuilder), nil
}

func clusterBuilderToBuildpackRecords(builder *buildv1alpha2.ClusterBuilder) []BuildpackRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&builder.ObjectMeta)
	buildpackRecords := make([]BuildpackRecord, 0, len(builder.Status.Order))
	for i, orderEntry := range builder.Status.Order {
		currentRecord := BuildpackRecord{
			Name:      orderEntry.Group[0].Id,
			Position:  i + 1,
			Stack:     builder.Spec.Stack.Name,
			Version:   orderEntry.Group[0].Version,
			CreatedAt: builder.CreationTimestamp.UTC().Format(TimestampFormat),
			UpdatedAt: updatedAtTime,
		}
		buildpackRecords = append(buildpackRecords, currentRecord)
	}
	return buildpackRecords
}
