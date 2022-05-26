package v1alpha1_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFRouteMutatingWebhook Unit Tests", func() {
	const (
		cfDomainGUID     = "test-domain-guid"
		cfRouteGUID      = "test-route-guid"
		cfDomainLabelKey = "korifi.cloudfoundry.org/domain-guid"
		cfRouteLabelKey  = "korifi.cloudfoundry.org/route-guid"
		namespace        = "default"
	)

	When("there are no existing labels on the CFRoute record", func() {
		It("should add new domain-guid and route-guid labels", func() {
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
					DomainRef: v1.ObjectReference{
						Name:      cfDomainGUID,
						Namespace: namespace,
					},
				},
			}

			cfRoute.Default()
			Expect(cfRoute.ObjectMeta.Labels).To(HaveKeyWithValue(cfRouteLabelKey, cfRouteGUID))
			Expect(cfRoute.ObjectMeta.Labels).To(HaveKeyWithValue(cfDomainLabelKey, cfDomainGUID))
		})
	})

	When("there are other existing labels on the CFRoute record", func() {
		It("should preserve the other labels", func() {
			cfRoute := &korifiv1alpha1.CFRoute{
				TypeMeta: metav1.TypeMeta{
					Kind:       "CFRoute",
					APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      cfRouteGUID,
					Namespace: namespace,
					Labels: map[string]string{
						"anotherLabel": "route-label",
					},
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					DomainRef: v1.ObjectReference{
						Name:      cfDomainGUID,
						Namespace: namespace,
					},
				},
			}

			cfRoute.Default()
			Expect(cfRoute.ObjectMeta.Labels).To(HaveLen(3), "Unexpected number of labels")
			Expect(cfRoute.ObjectMeta.Labels).To(HaveKeyWithValue("anotherLabel", "route-label"))
		})
	})
})
