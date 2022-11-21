package k8s_test

import (
	"context"

	"code.cloudfoundry.org/korifi/tools/k8s"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("AddFinalizer", func() {
	var (
		log    logr.Logger
		secret *corev1.Secret
		ctx    context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		log = logr.New(logr.Discard().GetSink())
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "my-secret", Namespace: testNamespace.Name},
			StringData: map[string]string{"foo": "bar"},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())
	})

	JustBeforeEach(func() {
		Expect(k8s.AddFinalizer(ctx, log, k8sClient, secret, "foo.com/bar")).To(Succeed())
	})

	It("has the finalizer set", func() {
		Eventually(func(g Gomega) {
			var fetchedSecret corev1.Secret
			g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), &fetchedSecret)).To(Succeed())
			g.Expect(fetchedSecret.Finalizers).To(ConsistOf("foo.com/bar"))
		}).Should(Succeed())
	})

	When("adding the same finalizer again", func() {
		BeforeEach(func() {
			Expect(k8s.AddFinalizer(ctx, log, k8sClient, secret, "foo.com/bar")).To(Succeed())
		})

		It("still has the finalizer set", func() {
			Consistently(func(g Gomega) {
				var fetchedSecret corev1.Secret
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), &fetchedSecret)).To(Succeed())
				g.Expect(fetchedSecret.Finalizers).To(ConsistOf("foo.com/bar"))
			}).Should(Succeed())
		})
	})
})
