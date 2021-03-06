package integration_test

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("CFRouteMutatingWebhook Integration Tests", func() {
	When("a CFRoute record is created", func() {
		const (
			cfDomainLabelKey = "korifi.cloudfoundry.org/domain-guid"
			cfRouteLabelKey  = "korifi.cloudfoundry.org/route-guid"
			namespace        = "default"
		)

		var (
			cfDomainGUID string
			cfRouteGUID  string
		)

		It("should add new domain-guid and route-guid labels", func() {
			testCtx := context.Background()

			cfDomainGUID = GenerateGUID()
			cfRouteGUID = GenerateGUID()

			cfDomain := &korifiv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfDomainGUID,
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFDomainSpec{
					Name: "example.com",
				},
			}
			Expect(k8sClient.Create(testCtx, cfDomain)).To(Succeed())

			cfRoute := &korifiv1alpha1.CFRoute{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFRoute",
					APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfRouteGUID,
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host: "my-host",
					DomainRef: v1.ObjectReference{
						Name:      cfDomainGUID,
						Namespace: namespace,
					},
				},
			}
			Expect(k8sClient.Create(testCtx, cfRoute)).To(Succeed())

			cfRouteLookupKey := types.NamespacedName{Name: cfRouteGUID, Namespace: namespace}
			createdCFRoute := new(korifiv1alpha1.CFRoute)

			Eventually(func() map[string]string {
				err := k8sClient.Get(testCtx, cfRouteLookupKey, createdCFRoute)
				if err != nil {
					return nil
				}
				return createdCFRoute.ObjectMeta.Labels
			}).ShouldNot(BeEmpty(), "CFRoute resource does not have any metadata.labels")

			Expect(createdCFRoute.ObjectMeta.Labels).To(HaveKeyWithValue(cfDomainLabelKey, cfDomainGUID))
			Expect(createdCFRoute.ObjectMeta.Labels).To(HaveKeyWithValue(cfRouteLabelKey, cfRouteGUID))
			Expect(k8sClient.Delete(testCtx, cfRoute)).To(Succeed())
		})
	})
})
