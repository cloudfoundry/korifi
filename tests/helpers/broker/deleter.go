package broker

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CatalogDeleter struct {
	rootNamespace        string
	catalogLabelSelector client.MatchingLabels
}

func NewCatalogDeleter(rootNamespace string) *CatalogDeleter {
	return &CatalogDeleter{rootNamespace: rootNamespace}
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

	ctx := context.Background()

	config, err := controllerruntime.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	servicePlans := &korifiv1alpha1.CFServicePlanList{}
	Expect(k8sClient.List(ctx, servicePlans, client.InNamespace(d.rootNamespace), d.catalogLabelSelector)).To(Succeed())
	for _, plan := range servicePlans.Items {
		serviceBindings := &korifiv1alpha1.CFServiceBindingList{}
		Expect(k8sClient.List(ctx, serviceBindings, client.MatchingLabels{
			korifiv1alpha1.PlanGUIDLabelKey: plan.Name,
		})).To(Succeed())
		for _, binding := range serviceBindings.Items {
			Expect(k8s.PatchResource(ctx, k8sClient, &binding, func() {
				binding.Finalizers = nil
			})).To(Succeed())

			Expect(k8sClient.Delete(ctx, &binding)).To(Succeed())
		}

		serviceInstances := &korifiv1alpha1.CFServiceInstanceList{}
		Expect(k8sClient.List(ctx, serviceInstances)).To(Succeed())
		for _, instance := range serviceInstances.Items {
			if instance.Spec.PlanGUID != plan.Name {
				continue
			}
			Expect(k8s.Patch(ctx, k8sClient, &instance, func() {
				instance.Finalizers = nil
			})).To(Succeed())
			Expect(k8sClient.Delete(ctx, &instance)).To(Succeed())
		}

		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, &korifiv1alpha1.CFServiceBroker{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: d.rootNamespace,
				Name:      plan.Labels[korifiv1alpha1.RelServiceBrokerGUIDLabel],
			},
		}))).To(Succeed())
	}

	Expect(k8sClient.DeleteAllOf(
		ctx,
		&korifiv1alpha1.CFServiceOffering{},
		client.InNamespace(d.rootNamespace),
		d.catalogLabelSelector,
	)).To(Succeed())

	Expect(k8sClient.DeleteAllOf(
		ctx,
		&korifiv1alpha1.CFServicePlan{},
		client.InNamespace(d.rootNamespace),
		d.catalogLabelSelector,
	)).To(Succeed())
}
