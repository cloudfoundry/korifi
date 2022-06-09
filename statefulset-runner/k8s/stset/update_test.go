package stset_test

import (
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/k8sfakes"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset/stsetfakes"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	"code.cloudfoundry.org/lager"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Update", func() {
	var (
		logger     lager.Logger
		client     *k8sfakes.FakeClient
		pdbUpdater *stsetfakes.FakePodDisruptionBudgetUpdater

		updatedLRP *eiriniv1.LRP
		st         *appsv1.StatefulSet
		err        error
	)

	BeforeEach(func() {
		logger = tests.NewTestLogger("handler-test")

		client = new(k8sfakes.FakeClient)
		pdbUpdater = new(stsetfakes.FakePodDisruptionBudgetUpdater)

		updatedLRP = &eiriniv1.LRP{
			Spec: eiriniv1.LRPSpec{
				GUID:      "guid_1234",
				Version:   "version_1234",
				AppName:   "baldur",
				SpaceName: "space-foo",
				Instances: 5,
				Image:     "new/image",
			},
		}

		replicas := int32(3)
		st = &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "baldur",
				Namespace: "the-namespace",
				Annotations: map[string]string{
					stset.AnnotationProcessGUID: "Baldur-guid",
				},
			},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{Name: "another-container", Image: "another/image"},
							{Name: stset.ApplicationContainerName, Image: "old/image"},
						},
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		updater := stset.NewUpdater(logger, client, pdbUpdater)
		err = updater.Update(ctx, updatedLRP, st)
	})

	It("succeeds", func() {
		Expect(err).NotTo(HaveOccurred())
	})

	It("updates the statefulset", func() {
		Expect(client.PatchCallCount()).To(Equal(1))

		_, obj, _, _ := client.PatchArgsForCall(0)
		Expect(obj).To(BeAssignableToTypeOf(&appsv1.StatefulSet{}))
		st := obj.(*appsv1.StatefulSet)

		Expect(st.Namespace).To(Equal("the-namespace"))
		Expect(st.GetAnnotations()).NotTo(HaveKey("another"))
		Expect(*st.Spec.Replicas).To(Equal(int32(5)))
		Expect(st.Spec.Template.Spec.Containers[0].Image).To(Equal("another/image"))
		Expect(st.Spec.Template.Spec.Containers[1].Image).To(Equal("new/image"))
	})

	It("updates the pod disruption budget", func() {
		Expect(pdbUpdater.UpdateCallCount()).To(Equal(1))
		_, actualStatefulSet, actualLRP := pdbUpdater.UpdateArgsForCall(0)
		Expect(actualStatefulSet.Namespace).To(Equal("the-namespace"))
		Expect(actualStatefulSet.Name).To(Equal("baldur"))
		Expect(actualLRP).To(Equal(updatedLRP))
	})

	When("updating the pod disruption budget fails", func() {
		BeforeEach(func() {
			pdbUpdater.UpdateReturns(errors.New("update-error"))
		})

		It("returns an error", func() {
			Expect(err).To(MatchError(ContainSubstring("update-error")))
		})
	})

	When("the image is missing", func() {
		BeforeEach(func() {
			updatedLRP.Spec.Image = ""
		})

		It("succeeds", func() {
			Expect(err).NotTo(HaveOccurred())
		})

		It("doesn't reset the image", func() {
			Expect(client.PatchCallCount()).To(Equal(1))

			_, obj, _, _ := client.PatchArgsForCall(0)
			Expect(obj).To(BeAssignableToTypeOf(&appsv1.StatefulSet{}))
			st := obj.(*appsv1.StatefulSet)
			Expect(st.Spec.Template.Spec.Containers[1].Image).To(Equal("old/image"))
		})
	})

	When("update fails", func() {
		BeforeEach(func() {
			client.PatchReturns(errors.New("boom"))
		})

		It("should return a meaningful message", func() {
			Expect(err).To(MatchError(ContainSubstring("failed to patch statefulset")))
		})
	})
})
