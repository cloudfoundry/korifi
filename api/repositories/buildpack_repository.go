package repositories

import (
	"context"
	"fmt"

	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	"k8s.io/apimachinery/pkg/types"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=kpack.io,resources=clusterbuilders,verbs=get;list;watch;
//+kubebuilder:rbac:groups=kpack.io,resources=clusterbuilders/status,verbs=get

type BuildpackRepository struct {
	privilegedClient  client.Client
	userClientFactory UserK8sClientFactory
	authEnabled       bool
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

func NewBuildpackRepository(
	privilegedClient client.Client,
	userClientFactory UserK8sClientFactory,
	authEnabled bool,
) *BuildpackRepository {
	return &BuildpackRepository{
		privilegedClient:  privilegedClient,
		userClientFactory: userClientFactory,
		authEnabled:       authEnabled,
	}
}

func (r *BuildpackRepository) GetBuildpacksForBuilder(ctx context.Context, authInfo authorization.Info, builderName string) ([]BuildpackRecord, error) {
	clusterBuilder := &buildv1alpha2.ClusterBuilder{}

	if !r.authEnabled {
		err := r.privilegedClient.Get(ctx, types.NamespacedName{Name: builderName}, clusterBuilder)
		if err != nil {
			return []BuildpackRecord{}, err
		}
	} else {
		userClient, err := r.userClientFactory.BuildClient(authInfo)
		if err != nil {
			return []BuildpackRecord{}, fmt.Errorf("failed to build user client: %w", err)
		}
		err = userClient.Get(ctx, types.NamespacedName{Name: builderName}, clusterBuilder)
		if err != nil {
			return []BuildpackRecord{}, err
		}
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
