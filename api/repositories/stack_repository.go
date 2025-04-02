package repositories

import (
	"context"
	"fmt"
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/BooleanCat/go-functional/v2/it"
	"k8s.io/apimachinery/pkg/api/meta"
)

const (
	StackResourceType = "Stack"
)

type StackRepository struct {
	klient        Klient
	builderName   string
	rootNamespace string
}

type StackRecord struct {
	GUID        string
	CreatedAt   time.Time
	UpdatedAt   *time.Time
	Name        string
	Description string
}

func NewStackRepository(
	klient Klient,
	builderName string,
	rootNamespace string,
) *StackRepository {
	return &StackRepository{
		klient:        klient,
		builderName:   builderName,
		rootNamespace: rootNamespace,
	}
}

func (r *StackRepository) ListStacks(ctx context.Context, authInfo authorization.Info) ([]StackRecord, error) {
	builderInfo := &korifiv1alpha1.BuilderInfo{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      r.builderName,
		},
	}

	err := r.klient.Get(ctx, builderInfo)
	if err != nil {
		return nil, apierrors.FromK8sError(err, StackResourceType)
	}

	if !meta.IsStatusConditionTrue(builderInfo.Status.Conditions, korifiv1alpha1.StatusConditionReady) {
		return nil, apierrors.NewResourceNotReadyError(fmt.Errorf("BuilderInfo %q not ready", r.builderName))
	}

	return builderInfoToStackRecords(*builderInfo), nil
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
