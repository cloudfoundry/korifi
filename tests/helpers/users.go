package helpers

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
)

type ServiceAccountFactory struct {
	k8sClient     client.Client
	rootNamespace string
}

func NewServiceAccountFactory(rootNamespace string) *ServiceAccountFactory {
	GinkgoHelper()

	config, err := controllerruntime.GetConfig()
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())

	return &ServiceAccountFactory{
		k8sClient:     k8sClient,
		rootNamespace: rootNamespace,
	}
}

func (f *ServiceAccountFactory) CreateServiceAccount(name string) string {
	GinkgoHelper()

	Expect(f.k8sClient.Create(context.Background(), &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: f.rootNamespace,
			Name:      name,
		},
	})).To(Succeed())

	serviceAccountSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: f.rootNamespace,
			Name:      name,
			Annotations: map[string]string{
				corev1.ServiceAccountNameKey: name,
			},
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}
	Expect(f.k8sClient.Create(context.Background(), serviceAccountSecret)).To(Succeed())

	Eventually(func(g Gomega) {
		g.Expect(f.k8sClient.Get(
			context.Background(),
			client.ObjectKeyFromObject(serviceAccountSecret),
			serviceAccountSecret,
		)).To(Succeed())
		g.Expect(serviceAccountSecret.Data).To(HaveKey(corev1.ServiceAccountTokenKey))
	}).Should(Succeed())

	return string(serviceAccountSecret.Data[corev1.ServiceAccountTokenKey])
}

func (f *ServiceAccountFactory) DeleteServiceAccount(name string) {
	GinkgoHelper()

	Expect(f.k8sClient.Delete(context.Background(), &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: f.rootNamespace,
			Name:      name,
		},
	})).To(Succeed())
}
