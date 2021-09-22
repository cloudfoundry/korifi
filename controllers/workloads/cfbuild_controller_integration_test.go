package controllers_test

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	buildv1alpha1 "github.com/pivotal/kpack/pkg/apis/build/v1alpha1"
	"github.com/sclevine/spec"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = AddToTestSuite("CFBuildReconciler", testCFBuildReconcilerIntegration)

func testCFBuildReconcilerIntegration(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)

	when("a new kpack image resource is created", func() {

		var kpackImageCR *buildv1alpha1.Image

		it.Before(func() {
			kpackImageCR = &buildv1alpha1.Image{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kpack-image-name",
					Namespace: "default",
				},
				Spec: buildv1alpha1.ImageSpec{
					Tag: "kpack-image-tag",
					Builder: corev1.ObjectReference{
						Kind:       "ClusterBuilder",
						Name:       "my-sample-builder", // TODO: cf-for-k8s makes a builder per-app
						APIVersion: "kpack.io/v1alpha1",
					},
					ServiceAccount: "kpack-service-account", // TODO: this is hardcoded too! You need a serviceAccount w/ secrets with this name in every namespace you build in.
					Source: buildv1alpha1.SourceConfig{
						Registry: &buildv1alpha1.Registry{
							Image: "source-image-name",
						},
						SubPath: "",
					},
				},
			}
		})

		it("should succeed", func() {
			g.Expect(k8sClient.Create(context.Background(), kpackImageCR)).To(Succeed())
			g.Expect(k8sClient.Delete(context.Background(), kpackImageCR)).To(Succeed())
		})
	})
}
