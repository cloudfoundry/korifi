package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	corev1 "k8s.io/api/core/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	PodResourceType        = "Pod"
	PodMetricsResourceType = "Pod Metrics"
)

type MetricsRepo struct {
	userClientFactory authorization.UserK8sClientFactory
}

func NewMetricsRepo(userClientFactory authorization.UserK8sClientFactory) *MetricsRepo {
	return &MetricsRepo{
		userClientFactory: userClientFactory,
	}
}

type PodMetrics struct {
	Pod     corev1.Pod
	Metrics metricsv1beta1.PodMetrics
}

func (r *MetricsRepo) GetMetrics(ctx context.Context, authInfo authorization.Info, namespace string, podSelector client.MatchingLabels) ([]PodMetrics, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	podList := &corev1.PodList{}
	err = userClient.List(ctx, podList, client.InNamespace(namespace), podSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", apierrors.FromK8sError(err, PodResourceType))
	}

	var res []PodMetrics
	for _, pod := range podList.Items {
		metrics := &metricsv1beta1.PodMetrics{}
		_ = userClient.Get(ctx, client.ObjectKey{
			Name:      pod.Name,
			Namespace: namespace,
		}, metrics)

		res = append(res, PodMetrics{Pod: pod, Metrics: *metrics})
	}

	return res, nil
}
