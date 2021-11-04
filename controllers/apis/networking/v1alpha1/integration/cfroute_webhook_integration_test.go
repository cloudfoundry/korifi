package integration_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"
	. "code.cloudfoundry.org/cf-k8s-controllers/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("CFRouteMutatingWebhook Integration Tests", func() {
	When("a CFRoute record is created", func() {
		const (
			cfDomainLabelKey = "networking.cloudfoundry.org/domain-guid"
			cfRouteLabelKey  = "networking.cloudfoundry.org/route-guid"
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

			cfRoute := &v1alpha1.CFRoute{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFRoute",
					APIVersion: v1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfRouteGUID,
					Namespace: namespace,
				},
				Spec: v1alpha1.CFRouteSpec{
					DomainRef: v1.LocalObjectReference{
						Name: cfDomainGUID,
					},
				},
			}
			Expect(k8sClient.Create(testCtx, cfRoute)).To(Succeed())

			cfRouteLookupKey := types.NamespacedName{Name: cfRouteGUID, Namespace: namespace}
			createdCFRoute := new(v1alpha1.CFRoute)

			Eventually(func() map[string]string {
				err := k8sClient.Get(testCtx, cfRouteLookupKey, createdCFRoute)
				if err != nil {
					return nil
				}
				return createdCFRoute.ObjectMeta.Labels
			}, 10*time.Second, 250*time.Millisecond).ShouldNot(BeEmpty(), "CFRoute resource does not have any metadata.labels")

			Expect(createdCFRoute.ObjectMeta.Labels).To(HaveKeyWithValue(cfDomainLabelKey, cfDomainGUID))
			Expect(createdCFRoute.ObjectMeta.Labels).To(HaveKeyWithValue(cfRouteLabelKey, cfRouteGUID))
			k8sClient.Delete(testCtx, cfRoute)
		})
	})
})
