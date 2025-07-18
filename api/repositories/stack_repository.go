package repositories

import (
	"context"
	"fmt"
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
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

type ListStacksMessage struct {
	Pagination Pagination
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

func (r *StackRepository) ListStacks(ctx context.Context, authInfo authorization.Info, message ListStacksMessage) (ListResult[StackRecord], error) {
	builderInfo := &korifiv1alpha1.BuilderInfo{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: r.rootNamespace,
			Name:      r.builderName,
		},
	}

	err := r.klient.Get(ctx, builderInfo)
	if err != nil {
		return ListResult[StackRecord]{}, apierrors.FromK8sError(err, StackResourceType)
	}

	if !meta.IsStatusConditionTrue(builderInfo.Status.Conditions, korifiv1alpha1.StatusConditionReady) {
		return ListResult[StackRecord]{}, apierrors.NewResourceNotReadyError(fmt.Errorf("BuilderInfo %q not ready", r.builderName))
	}

	stackRecords := builderInfoToStackRecords(*builderInfo)

	recordsPage := descriptors.SinglePage(stackRecords, len(stackRecords))
	if !message.Pagination.IsZero() {
		var err error
		recordsPage, err = descriptors.GetPage(stackRecords, message.Pagination.PerPage, message.Pagination.Page)
		if err != nil {
			return ListResult[StackRecord]{}, fmt.Errorf("failed to page spaces list: %w", err)
		}
	}

	return ListResult[StackRecord]{
		PageInfo: recordsPage.PageInfo,
		Records:  recordsPage.Items,
	}, nil
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
