package broker

import (
	"context"
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CatalogDeleter struct {
	ctx                  context.Context
	k8sClient            client.Client
	rootNamespace        string
	catalogLabelSelector client.MatchingLabels
	maxRetries           int
}

func NewCatalogDeleter(rootNamespace string) *CatalogDeleter {
	ctx := context.Background()

	config, err := controllerruntime.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	return &CatalogDeleter{
		ctx:           ctx,
		k8sClient:     k8sClient,
		rootNamespace: rootNamespace,
		maxRetries:    50,
	}
}

func (d *CatalogDeleter) ForBrokerGUID(brokerGUID string) *CatalogDeleter {
	d.catalogLabelSelector = client.MatchingLabels{
		korifiv1alpha1.RelServiceBrokerGUIDLabel: brokerGUID,
	}

	return d
}

func (d *CatalogDeleter) ForBrokerName(brokerName string) *CatalogDeleter {
	d.catalogLabelSelector = client.MatchingLabels{
		korifiv1alpha1.RelServiceBrokerNameLabel: brokerName,
	}

	return d
}

func (d *CatalogDeleter) Delete() {
	GinkgoHelper()

	servicePlans := &korifiv1alpha1.CFServicePlanList{}
	Expect(d.k8sClient.List(d.ctx, servicePlans, client.InNamespace(d.rootNamespace), d.catalogLabelSelector)).To(Succeed())
	for _, plan := range servicePlans.Items {
		Expect(d.cleanupBindings(plan.Name)).To(Succeed())
		Expect(d.cleanupInsances(plan.Name)).To(Succeed())
		Expect(client.IgnoreNotFound(d.k8sClient.Delete(d.ctx, &korifiv1alpha1.CFServiceBroker{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: d.rootNamespace,
				Name:      plan.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel],
			},
		}))).To(Succeed())
	}

	Expect(d.k8sClient.DeleteAllOf(
		d.ctx,
		&korifiv1alpha1.CFServiceOffering{},
		client.InNamespace(d.rootNamespace),
		d.catalogLabelSelector,
	)).To(Succeed())

	Expect(d.k8sClient.DeleteAllOf(
		d.ctx,
		&korifiv1alpha1.CFServicePlan{},
		client.InNamespace(d.rootNamespace),
		d.catalogLabelSelector,
	)).To(Succeed())
}

func (d *CatalogDeleter) cleanupBindings(planName string) error {
	for retries := 0; retries < d.maxRetries; retries++ {
		serviceBindings := &korifiv1alpha1.CFServiceBindingList{}
		Expect(d.k8sClient.List(d.ctx, serviceBindings, client.MatchingLabels{
			korifiv1alpha1.PlanGUIDLabelKey: planName,
		})).To(Succeed())

		if len(serviceBindings.Items) == 0 {
			return nil
		}

		for _, binding := range serviceBindings.Items {
			forceDelete(d.ctx, d.k8sClient, &binding)
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("failed to clean up service bindings for plan %q", planName)
}

func (d *CatalogDeleter) cleanupInsances(planName string) error {
	for retries := 0; retries < d.maxRetries; retries++ {
		serviceInstances := &korifiv1alpha1.CFServiceInstanceList{}
		Expect(d.k8sClient.List(d.ctx, serviceInstances)).To(Succeed())

		instancesForPlan := itx.FromSlice(serviceInstances.Items).
			Filter(func(instance korifiv1alpha1.CFServiceInstance) bool {
				return instance.Spec.PlanGUID == planName
			}).Collect()

		if len(instancesForPlan) == 0 {
			return nil
		}

		for _, instance := range instancesForPlan {
			forceDelete(d.ctx, d.k8sClient, &instance)
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("failed to clean up service bindings for plan %q", planName)
}

func forceDelete[T any, PT k8s.ObjectWithDeepCopy[T]](ctx context.Context, k8sClient client.Client, obj PT) {
	Expect(k8s.PatchResource(ctx, k8sClient, obj, func() {
		obj.SetFinalizers(nil)
	})).To(Succeed())

	Expect(k8sClient.Delete(ctx, obj)).To(Succeed())
}
