package k8s_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Kubernetes Patch", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("Patch", func() {
		Describe("objects with Status", func() {
			var (
				pod        *corev1.Pod
				patchedPod *corev1.Pod
				patchErr   error
			)
			BeforeEach(func() {
				pod = &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace.Name,
						Name:      uuid.NewString(),
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:    "foo",
							Image:   "busybox",
							Command: []string{"echo", "hi"},
						}},
					},
					Status: corev1.PodStatus{},
				}
				Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			})

			JustBeforeEach(func() {
				patchErr = k8s.Patch(ctx, k8sClient, pod, func() {
					pod.Spec.Containers[0].Image = "alpine"
					pod.Status.Message = "hello"
				})

				patchedPod = &corev1.Pod{}
				err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(pod), patchedPod)
				Expect(err).NotTo(HaveOccurred())
			})

			It("patches the object via the k8s client", func() {
				Expect(patchErr).NotTo(HaveOccurred())
				Expect(patchedPod.Spec.Containers[0].Image).To(Equal("alpine"))
			})

			When("patching the object fails", func() {
				var cancel context.CancelFunc

				BeforeEach(func() {
					ctx, cancel = context.WithTimeout(ctx, -1*time.Second)
				})

				AfterEach(func() {
					cancel()
				})

				It("returns the error", func() {
					Expect(patchErr).To(MatchError(ContainSubstring("context deadline exceeded")))
				})
			})

			It("patches the object status via the k8s client", func() {
				Expect(patchErr).NotTo(HaveOccurred())
				Expect(patchedPod.Status.Message).To(Equal("hello"))
			})
		})

		Describe("objects without status", func() {
			var (
				secret        *corev1.Secret
				patchedSecret *corev1.Secret
			)

			BeforeEach(func() {
				secret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace.Name,
						Name:      uuid.NewString(),
					},
					Data: map[string][]byte{
						"foo": []byte("bar"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			})

			JustBeforeEach(func() {
				Expect(k8s.Patch(ctx, k8sClient, secret, func() {
					secret.Data["jim"] = []byte("bob")
				})).To(Succeed())

				patchedSecret = &corev1.Secret{}
				err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(secret), patchedSecret)
				Expect(err).NotTo(HaveOccurred())
			})

			It("patches the object via the k8s client", func() {
				Expect(patchedSecret.Data).To(Equal(map[string][]byte{
					"foo": []byte("bar"),
					"jim": []byte("bob"),
				}))
			})
		})
	})

	Describe("PatchSpec", func() {
		var (
			pod        *corev1.Pod
			patchedPod *corev1.Pod
			patchErr   error
		)
		BeforeEach(func() {
			pod = &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testNamespace.Name,
					Name:      uuid.NewString(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "foo",
						Image:   "busybox",
						Command: []string{"echo", "hi"},
					}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())

			createdPod := pod.DeepCopy()
			pod.Status = corev1.PodStatus{
				Message: "hello",
			}
			Expect(k8sClient.Status().Patch(ctx, pod, client.MergeFrom(createdPod))).To(Succeed())
		})

		JustBeforeEach(func() {
			patchErr = k8s.PatchSpec(ctx, k8sClient, pod, func() {
				pod.Spec.Containers[0].Image = "alpine"
				pod.Status.Message = "bye"
			})

			patchedPod = &corev1.Pod{}
			err := k8sClient.Get(context.Background(), client.ObjectKeyFromObject(pod), patchedPod)
			Expect(err).NotTo(HaveOccurred())
		})

		It("patches the object via the k8s client", func() {
			Expect(patchErr).NotTo(HaveOccurred())
			Expect(patchedPod.Spec.Containers[0].Image).To(Equal("alpine"))
		})

		When("patching the object fails", func() {
			var cancel context.CancelFunc

			BeforeEach(func() {
				ctx, cancel = context.WithTimeout(ctx, -1*time.Second)
			})

			AfterEach(func() {
				cancel()
			})

			It("returns the error", func() {
				Expect(patchErr).To(MatchError(ContainSubstring("context deadline exceeded")))
			})
		})

		It("does not patch the status", func() {
			Expect(patchedPod.Status.Message).To(Equal("hello"))
		})
	})
})
