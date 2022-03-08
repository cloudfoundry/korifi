package e2e_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = FDescribe("RoleLatency", func() {
	var (
		namespace   string
		ctx         context.Context
		rolebinding string
		userName    string
		config      *rest.Config
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error

		config, err = controllerruntime.GetConfig()
		Expect(err).NotTo(HaveOccurred())

		namespace = generateGUID("namespace")
		Expect(k8sClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}})).To(Succeed())
	})

	lim := 100

	doIt := func() {
		for i := 0; i < lim; i++ {
			userName = generateGUID("user")
			localConfig := rest.CopyConfig(config)
			localConfig.Impersonate.UserName = userName

			userClient, err := client.NewWithWatch(config, client.Options{
				Scheme: scheme.Scheme,
				Mapper: k8sClient.RESTMapper(),
			})
			Expect(err).NotTo(HaveOccurred())

			rolebinding = generateGUID("rolebinding")
			Expect(k8sClient.Create(ctx, &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: rolebinding},
				Subjects: []rbacv1.Subject{{
					Kind: rbacv1.UserKind,
					Name: userName + "a",
				}},
				RoleRef: rbacv1.RoleRef{
					Kind: "ClusterRole",
					Name: "cluster-admin",
				},
			})).To(Succeed())
			Expect(userClient.List(ctx, &corev1.PodList{}, client.InNamespace(namespace))).To(Succeed())
		}
	}

	It("1", func() { doIt() })
	It("2", func() { doIt() })
	It("3", func() { doIt() })
	It("4", func() { doIt() })
	It("5", func() { doIt() })
	It("6", func() { doIt() })
	It("7", func() { doIt() })
})
