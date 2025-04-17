package repositories

import (
	"context"
	"fmt"
	"slices"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"github.com/BooleanCat/go-functional/v2/it"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	PodResourceType        = "Pod"
	PodMetricsResourceType = "Pod Metrics"
)

type MetricsRepo struct {
	klient Klient
}

func NewMetricsRepo(klient Klient) *MetricsRepo {
	return &MetricsRepo{
		klient: klient,
	}
}

type PodMetrics struct {
	Pod     corev1.Pod
	Metrics metricsv1beta1.PodMetrics
}

func (r *MetricsRepo) GetMetrics(ctx context.Context, authInfo authorization.Info, namespace string, podSelector client.MatchingLabels) ([]PodMetrics, error) {
	podList := &corev1.PodList{}
	err := r.klient.List(ctx, podList, InNamespace(namespace), WithLabels{Selector: labels.SelectorFromValidatedSet(map[string]string(podSelector))})
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
		_ = r.klient.Get(ctx, metrics)
		return PodMetrics{Pod: pod, Metrics: *metrics}
	})), nil
}
