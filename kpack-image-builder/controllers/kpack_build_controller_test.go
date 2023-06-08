package controllers_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kpackv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("KpackBuildReconciler", func() {
	var (
		namespaceGUID   string
		build           kpackv1alpha2.Build
		deleteCallCount int
		setImage        bool
	)

	BeforeEach(func() {
		namespaceGUID = PrefixedGUID("namespace")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespaceGUID,
			},
		})).To(Succeed())

		setImage = true
		deleteCallCount = fakeImageDeleter.DeleteCallCount()
	})

	Context("Korifi builds", func() {
		BeforeEach(func() {
			build = kpackv1alpha2.Build{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: namespaceGUID,
					Labels: map[string]string{
						"korifi.cloudfoundry.org/build-workload-name": "anything",
					},
				},
				Spec: kpackv1alpha2.BuildSpec{
					Tags: []string{
						"foo.reg/latest-image",
						"foo.reg/latest-image:bob",
					},
				},
			}
			Expect(k8sClient.Create(ctx, &build)).To(Succeed())
		})

		JustBeforeEach(func() {
			if setImage {
				buildOrig := build.DeepCopy()
				build.Status.LatestImage = "foo.reg/latest-image@sha256:abcdef123"
				Expect(k8sClient.Status().Patch(ctx, &build, client.MergeFrom(buildOrig))).To(Succeed())
			}

			Eventually(func(g Gomega) {
				gotBuild := kpackv1alpha2.Build{}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&build), &gotBuild)).To(Succeed())
				g.Expect(gotBuild.Finalizers).NotTo(BeEmpty())
			}).Should(Succeed())

			Expect(k8sClient.Delete(ctx, &build)).To(Succeed())
		})

		It("works", func() {
			Eventually(func(g Gomega) {
				g.Expect(fakeImageDeleter.DeleteCallCount()).To(Equal(deleteCallCount + 1))
				_, creds, imageRef, tagsToDelete := fakeImageDeleter.DeleteArgsForCall(deleteCallCount)
				g.Expect(creds.Namespace).To(Equal(namespaceGUID))
				g.Expect(creds.ServiceAccountName).To(Equal("builder-service-account"))
				g.Expect(imageRef).To(Equal(build.Status.LatestImage))
				g.Expect(tagsToDelete).To(ConsistOf("bob"))

				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&build), &build)
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
			}).Should(Succeed())
		})

		When("the image is not set", func() {
			BeforeEach(func() {
				setImage = false
			})

			It("doesn't try to delete the image", func() {
				Consistently(func(g Gomega) {
					g.Expect(fakeImageDeleter.DeleteCallCount()).To(Equal(deleteCallCount))
				}).Should(Succeed())
			})
		})

		When("deleting the image fails", func() {
			BeforeEach(func() {
				fakeImageDeleter.DeleteReturns(errors.New("bang"))
			})

			It("still deletes the build", func() {
				Eventually(func(g Gomega) {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&build), &build)
					g.Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				}).Should(Succeed())
			})
		})
	})
})
