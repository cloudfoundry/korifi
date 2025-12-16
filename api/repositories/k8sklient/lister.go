package k8sklient

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors/errors"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//counterfeiter:generate -o fake -fake-name DescriptorClient . DescriptorClient
type DescriptorClient interface {
	List(ctx context.Context, listObjectGVK schema.GroupVersionKind, opts ...client.ListOption) (descriptors.ResultSetDescriptor, error)
}

//counterfeiter:generate -o fake -fake-name ObjectListMapper . ObjectListMapper
type ObjectListMapper interface {
	GUIDsToObjectList(ctx context.Context, listObjectGVK schema.GroupVersionKind, orderedGUIDs []string) (client.ObjectList, error)
}

type DescriptorsBasedLister struct {
	descriptorClient DescriptorClient
	objectListMapper ObjectListMapper
}

func NewDescriptorsBasedLister(
	descriptorClient DescriptorClient,
	objectListMapper ObjectListMapper,
) *DescriptorsBasedLister {
	return &DescriptorsBasedLister{
		descriptorClient: descriptorClient,
		objectListMapper: objectListMapper,
	}
}

func (k *DescriptorsBasedLister) List(ctx context.Context, listObjectGVK schema.GroupVersionKind, listOpts repositories.ListOptions) (client.ObjectList, descriptors.PageInfo, error) {
	listResult, pageInfo, err := k.retryList(ctx, listObjectGVK, listOpts)
	if err != nil {
		return nil, descriptors.PageInfo{}, err
	}

	return listResult, pageInfo, nil
}

func (k *DescriptorsBasedLister) retryList(ctx context.Context, listObjectGVK schema.GroupVersionKind, listOpts repositories.ListOptions) (client.ObjectList, descriptors.PageInfo, error) {
	logger := logr.FromContextOrDiscard(ctx).WithName("k8sklient-list")

	var (
		listResult client.ObjectList
		pageInfo   descriptors.PageInfo
		err        error
	)

	err = retry.OnError(k8s.NewDefaultBackoff(), errors.IsObjectResolutionError, func() error {
		listResult, pageInfo, err = k.descriptorsBasedList(ctx, listObjectGVK, listOpts)
		if err != nil {
			logger.Info("failed to resolve objects, retrying", "error", err)
		}
		return err
	})

	return listResult, pageInfo, err
}

func (k *DescriptorsBasedLister) descriptorsBasedList(ctx context.Context, listObjectGVK schema.GroupVersionKind, listOpts repositories.ListOptions) (client.ObjectList, descriptors.PageInfo, error) {
	objectGUIDs, err := k.fetchObjectGUIDs(ctx, listObjectGVK, listOpts)
	if err != nil {
		return nil, descriptors.PageInfo{}, fmt.Errorf("failed to fetch object guids: %w", err)
	}

	var pageInfo descriptors.PageInfo
	objectGUIDs, pageInfo, err = pageGUIDs(objectGUIDs, listOpts)
	if err != nil {
		return nil, descriptors.PageInfo{}, fmt.Errorf("failed to page object guids: %w", err)
	}

	listResult, err := k.objectListMapper.GUIDsToObjectList(ctx, listObjectGVK, objectGUIDs)
	if err != nil {
		return nil, descriptors.PageInfo{}, err
	}

	return listResult, pageInfo, nil
}

func (k *DescriptorsBasedLister) fetchObjectGUIDs(ctx context.Context, listObjectGVK schema.GroupVersionKind, listOpts repositories.ListOptions) ([]string, error) {
	objectDescriptors, err := k.descriptorClient.List(ctx, listObjectGVK, listOpts.AsClientListOptions())
	if err != nil {
		return nil, fmt.Errorf("failed to list object descriptors: %w", err)
	}

	if listOpts.Sort != nil {
		if err = objectDescriptors.Sort(listOpts.Sort.By, listOpts.Sort.Desc); err != nil {
			return nil, fmt.Errorf("failed to sort object descriptors: %w", err)
		}
	}

	if listOpts.Filter != nil {
		if err = objectDescriptors.Filter(listOpts.Filter.By, listOpts.Filter.FilterFunc); err != nil {
			return nil, fmt.Errorf("failed to filter object descriptors: %w", err)
		}
	}

	objectGUIDs, err := objectDescriptors.GUIDs()
	if err != nil {
		return nil, fmt.Errorf("failed to get sorted object GUIDs: %w", err)
	}

	return objectGUIDs, nil
}

func pageGUIDs(objectGUIDs []string, listOpts repositories.ListOptions) ([]string, descriptors.PageInfo, error) {
	if listOpts.Paging == nil {
		return objectGUIDs, descriptors.SinglePageInfo(len(objectGUIDs), len(objectGUIDs)), nil
	}

	page, err := descriptors.GetPage(objectGUIDs, listOpts.Paging.PageSize, listOpts.Paging.PageNumber)
	if err != nil {
		return nil, descriptors.PageInfo{}, fmt.Errorf("failed to page object guids: %w", err)
	}

	return page.Items, page.PageInfo, nil
}
