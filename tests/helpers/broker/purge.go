package broker

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file

	"k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CatalogPurger struct {
	rootNamespace        string
	catalogLabelSelector client.MatchingLabels
}

func NewCatalogPurger(rootNamespace string) *CatalogPurger {
	return &CatalogPurger{rootNamespace: rootNamespace}
}

func (p *CatalogPurger) ForBrokerGUID(brokerGUID string) *CatalogPurger {
	p.catalogLabelSelector = client.MatchingLabels{
		korifiv1alpha1.RelServiceBrokerGUIDLabel: brokerGUID,
	}

	return p
}

func (p *CatalogPurger) ForBrokerName(brokerName string) *CatalogPurger {
	p.catalogLabelSelector = client.MatchingLabels{
		korifiv1alpha1.RelServiceBrokerNameLabel: brokerName,
	}

	return p
}

func (p *CatalogPurger) Purge() {
	GinkgoHelper()

	ctx := context.Background()

	config, err := controllerruntime.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	Expect(k8sClient.DeleteAllOf(
		ctx,
		&korifiv1alpha1.CFServiceOffering{},
		client.InNamespace(p.rootNamespace),
		p.catalogLabelSelector,
	)).To(Succeed())

	Expect(k8sClient.DeleteAllOf(
		ctx,
		&korifiv1alpha1.CFServicePlan{},
		client.InNamespace(p.rootNamespace),
		p.catalogLabelSelector,
	)).To(Succeed())
}
