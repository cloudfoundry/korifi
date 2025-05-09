package repositories_test

import (
	"fmt"
	"net/http"

	"code.cloudfoundry.org/korifi/api/repositories"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = FDescribe("Cursor", func() {
	var namespace *corev1.Namespace

	BeforeEach(func() {
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: uuid.NewString(),
			},
		}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

		for i := 0; i < 1000; i++ {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("secret-%d", i),
					Namespace: namespace.Name,
				},
				StringData: map[string]string{
					"key": "value",
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
		}
	})

	It("Lists secrets in uneven batches. Continue tokens can be reused across different client instances.", func() {
		secrets := &corev1.SecretList{}
		Expect(k8sClient.List(ctx, secrets, client.InNamespace(namespace.Name), client.Limit(900))).To(Succeed())
		Expect(secrets.Items).To(HaveLen(900))
		Expect(secrets.RemainingItemCount).To(PointTo(BeEquivalentTo(100)))

		k8sClient2, err := client.NewWithWatch(testEnv.Config, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient2.List(ctx, secrets, client.InNamespace(namespace.Name), client.Limit(20), client.Continue(secrets.Continue))).To(Succeed())
		Expect(secrets.Items).To(HaveLen(20))
		Expect(secrets.RemainingItemCount).To(PointTo(BeEquivalentTo(80)))

		k8sClient3, err := client.NewWithWatch(testEnv.Config, client.Options{Scheme: scheme.Scheme})
		Expect(err).NotTo(HaveOccurred())

		Expect(k8sClient3.List(ctx, secrets, client.InNamespace(namespace.Name), client.Limit(80), client.Continue(secrets.Continue))).To(Succeed())
		Expect(secrets.Items).To(HaveLen(80))
		Expect(secrets.RemainingItemCount).To(BeNil())
	})

	Describe("Seeking Client", func() {
		var seekingClient client.Client

		BeforeEach(func() {
			httpClient, err := rest.HTTPClientFor(testEnv.Config)
			Expect(err).NotTo(HaveOccurred())
			seekingClient, err = client.NewWithWatch(testEnv.Config, client.Options{
				Scheme: scheme.Scheme,
				HTTPClient: &http.Client{
					Transport: repositories.NewDiscardBodyRoundTripper(httpClient.Transport),
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})

		FIt("does something", func() {
			secrets := &corev1.SecretList{}
			Expect(seekingClient.List(ctx, secrets, client.InNamespace(namespace.Name), client.Limit(900))).To(Succeed())
			Expect(secrets.Items).To(HaveLen(900))
			Expect(secrets.RemainingItemCount).To(PointTo(BeEquivalentTo(100)))
		})
	})
})
