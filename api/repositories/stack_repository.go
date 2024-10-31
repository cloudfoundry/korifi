package repositories

import (
	"context"
	"fmt"
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/BooleanCat/go-functional/v2/it"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
)

const (
	StackResourceType = "Stack"
)

type StackRepository struct {
	builderName       string
	userClientFactory authorization.UserK8sClientFactory
	rootNamespace     string
}

type StackRecord struct {
	GUID        string
	CreatedAt   time.Time
	UpdatedAt   *time.Time
	Name        string
	Description string
}

func NewStackRepository(
	builderName string,
	userClientFactory authorization.UserK8sClientFactory,
	rootNamespace string,
) *StackRepository {
	return &StackRepository{
		builderName:       builderName,
		userClientFactory: userClientFactory,
		rootNamespace:     rootNamespace,
	}
}

func (r *StackRepository) ListStacks(ctx context.Context, authInfo authorization.Info) ([]StackRecord, error) {
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
		return nil, apierrors.FromK8sError(err, StackResourceType)
	}

	if !meta.IsStatusConditionTrue(builderInfo.Status.Conditions, korifiv1alpha1.StatusConditionReady) {
		return nil, apierrors.NewResourceNotReadyError(fmt.Errorf("BuilderInfo %q not ready", r.builderName))
	}

	return builderInfoToStackRecords(builderInfo), nil
}

func builderInfoToStackRecords(info korifiv1alpha1.BuilderInfo) []StackRecord {
	return slices.Collect(it.Map(slices.Values(info.Status.Stacks), func(s korifiv1alpha1.BuilderInfoStatusStack) StackRecord {
		return StackRecord{
			Name:        s.Name,
			Description: s.Description,
			CreatedAt:   s.CreationTimestamp.Time,
			UpdatedAt:   &s.UpdatedTimestamp.Time,
		}
	}))
}
