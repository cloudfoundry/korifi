package repositories_test

import (
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("MetadataPatch", func() {
	var (
		pod           *corev1.Pod
		metadataPatch repositories.MetadataPatch
	)

	Describe("Apply", func() {
		BeforeEach(func() {
			pod = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Labels: map[string]string{
						"not-to-be-modified-label": "immutable-label-value",
						"to-be-modified-label":     "original-label-value",
						"to-be-removed-label":      "label-value",
					},
					Annotations: map[string]string{
						"not-to-be-modified-annotation": "immutable-annotation-value",
						"to-be-modified-annotation":     "original-annotation-value",
						"to-be-removed-annotation":      "annotation-value",
					},
				},
			}

			metadataPatch = repositories.MetadataPatch{
				Labels: map[string]*string{
					"to-be-added-label":    tools.PtrTo("added-label-value"),
					"to-be-modified-label": tools.PtrTo("modified-label-value"),
					"to-be-removed-label":  nil,
				},
				Annotations: map[string]*string{
					"to-be-added-annotation":    tools.PtrTo("added-annotation-value"),
					"to-be-modified-annotation": tools.PtrTo("modified-annotation-value"),
					"to-be-removed-annotation":  nil,
				},
			}
		})

		JustBeforeEach(func() {
			metadataPatch.Apply(pod)
		})

		It("updates labels and annotations correctly", func() {
			Expect(pod.Labels).To(SatisfyAll(
				HaveLen(3),
				HaveKeyWithValue("to-be-added-label", "added-label-value"),
				HaveKeyWithValue("to-be-modified-label", "modified-label-value"),
				HaveKeyWithValue("not-to-be-modified-label", "immutable-label-value"),
			))
			Expect(pod.Annotations).To(SatisfyAll(
				HaveLen(3),
				HaveKeyWithValue("to-be-added-annotation", "added-annotation-value"),
				HaveKeyWithValue("to-be-modified-annotation", "modified-annotation-value"),
				HaveKeyWithValue("not-to-be-modified-annotation", "immutable-annotation-value"),
			))
		})
	})
})
