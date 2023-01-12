package networking_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	cfDomainLabelKey = "korifi.cloudfoundry.org/domain-guid"
	cfRouteLabelKey  = "korifi.cloudfoundry.org/route-guid"
)

var _ = FDescribe("CFRouteMutatingWebhook Integration Tests", func() {
	When("a CFRoute record is created", func() {
		var (
			cfDomain  *korifiv1alpha1.CFDomain
			cfRoute   *korifiv1alpha1.CFRoute
			createErr error
		)

		BeforeEach(func() {
			cfDomain = &korifiv1alpha1.CFDomain{
				ObjectMeta: metav1.ObjectMeta{
					Name:      GenerateGUID(),
					Namespace: namespace,
				},
				Spec: korifiv1alpha1.CFDomainSpec{
					Name: "example.com",
				},
			}
			Expect(k8sClient.Create(ctx, cfDomain)).To(Succeed())
			DeferCleanup(func() {
				Expect(k8sClient.Delete(ctx, cfDomain)).To(Succeed())
			})

			cfRoute = &korifiv1alpha1.CFRoute{
				ObjectMeta: metav1.ObjectMeta{
					Name:      GenerateGUID(),
					Namespace: namespace,
					Labels:    map[string]string{"foo": "bar"},
				},
				Spec: korifiv1alpha1.CFRouteSpec{
					Host: "my-host",
					DomainRef: v1.ObjectReference{
						Name:      cfDomain.Name,
						Namespace: namespace,
					},
				},
			}
		})

		JustBeforeEach(func() {
			createErr = k8sClient.Create(ctx, cfRoute)
			if createErr == nil {
				DeferCleanup(func() {
					Expect(k8sClient.Delete(ctx, cfRoute)).To(Succeed())
				})
			}
		})

		It("adds labels with guids of the domain and route", func() {
			Expect(createErr).NotTo(HaveOccurred())
			Expect(cfRoute.Labels).To(HaveKeyWithValue(cfDomainLabelKey, cfDomain.Name))
			Expect(cfRoute.Labels).To(HaveKeyWithValue(cfRouteLabelKey, cfRoute.Name))
		})

		It("preserves the other labels", func() {
			Expect(createErr).NotTo(HaveOccurred())
			Expect(cfRoute.Labels).To(HaveKeyWithValue("foo", "bar"))
		})

		It("sets domain owner ref", func() {
			Expect(createErr).NotTo(HaveOccurred())
			ownerRefs := cfRoute.GetOwnerReferences()
			Expect(ownerRefs).To(HaveLen(1))
			ref := ownerRefs[0]
			Expect(ref.Kind).To(Equal("CFDomain"))
			Expect(ref.UID).To(Equal(cfDomain.UID))
		})

		When("the domain does not exist", func() {
			BeforeEach(func() {
				cfRoute.Spec.DomainRef.Name += "xx"
			})

			It("fails creation", func() {
				Expect(createErr).To(MatchError(MatchRegexp("CFDomain.* not found")))
			})
		})
	})
})
