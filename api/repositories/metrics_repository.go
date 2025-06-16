package repositories

import (
	"context"
	"fmt"
	"slices"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"github.com/BooleanCat/go-functional/v2/it"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	PodResourceType        = "Pod"
	PodMetricsResourceType = "Pod Metrics"
)

type MetricsRepo struct {
	userClientFactory authorization.UserClientFactory
}

func NewMetricsRepo(userClientFactory authorization.UserClientFactory) *MetricsRepo {
	return &MetricsRepo{
		userClientFactory: userClientFactory,
	}
}

type PodMetrics struct {
	Pod     corev1.Pod
	Metrics metricsv1beta1.PodMetrics
}

func (r *MetricsRepo) GetMetrics(ctx context.Context, authInfo authorization.Info, app AppRecord, processGUID string) ([]PodMetrics, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	podList := &corev1.PodList{}
	err = userClient.List(ctx, podList,
		client.InNamespace(app.SpaceGUID),
		client.MatchingLabels{
			korifiv1alpha1.CFAppGUIDLabelKey: app.GUID,
			korifiv1alpha1.VersionLabelKey:   app.Revision,
			korifiv1alpha1.GUIDLabelKey:      processGUID,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", apierrors.FromK8sError(err, PodResourceType))
	}

	return slices.Collect(it.Map(slices.Values(podList.Items), func(pod corev1.Pod) PodMetrics {
		metrics := &metricsv1beta1.PodMetrics{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: pod.Namespace,
				Name:      pod.Name,
			},
		}
		_ = userClient.Get(ctx, client.ObjectKeyFromObject(metrics), metrics)
		return PodMetrics{Pod: pod, Metrics: *metrics}
	})), nil
}
