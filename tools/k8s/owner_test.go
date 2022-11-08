package k8s_test

import (
	"context"

	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = FDescribe("Owner", func() {
	var (
		owner       *corev1.Pod
		owned       *corev1.Secret
		setOwnerErr error
	)

	BeforeEach(func() {
		owner = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "owner",
				Namespace: testNamespace.Name,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name:  "c",
					Image: "alpine",
				}},
			},
		}

		Expect(k8sClient.Create(context.Background(), owner)).To(Succeed())

		owned = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "owned",
				Namespace: testNamespace.Name,
			},
		}
	})

	JustBeforeEach(func() {
		setOwnerErr = k8s.SetOwner(context.Background(), k8sClient, owner, owned)
	})

	It("sets the owner reference", func() {
		Expect(setOwnerErr).NotTo(HaveOccurred())
		Expect(owned.GetOwnerReferences()).To(ConsistOf(
			MatchFields(IgnoreExtras, Fields{
				"Name": Equal(owner.Name),
			}),
		))
	})

	When("getting the owner fails", func() {
		BeforeEach(func() {
			owner.Name = "i-do-not-exist"
		})

		It("returns an error", func() {
			Expect(setOwnerErr).To(HaveOccurred())
		})
	})

	When("setting the owner reference fails", func() {
		BeforeEach(func() {
			owner.Namespace = "default"
		})

		It("returns an error", func() {
			Expect(setOwnerErr).To(HaveOccurred())
		})
	})
})
