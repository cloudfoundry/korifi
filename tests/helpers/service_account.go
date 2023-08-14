package helpers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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

	_, serviceAccountToken := f.createServiceAccount(name)
	return serviceAccountToken
}

func (f *ServiceAccountFactory) CreateAdminServiceAccount(adminServiceAccount string) string {
	GinkgoHelper()

	serviceAccount, adminServiceAccountToken := f.createServiceAccount(adminServiceAccount)

	adminRoleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: f.rootNamespace,
			Name:      adminServiceAccount,
			Annotations: map[string]string{
				"cloudfoundry.org/propagate-cf-role": "true",
			},
		},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      adminServiceAccount,
			Namespace: f.rootNamespace,
		}},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: "korifi-controllers-admin",
		},
	}
	Expect(controllerutil.SetOwnerReference(serviceAccount, adminRoleBinding, scheme.Scheme)).To(Succeed())
	Expect(f.k8sClient.Create(context.Background(), adminRoleBinding)).To(Succeed())

	return adminServiceAccountToken
}

func (f *ServiceAccountFactory) createServiceAccount(name string) (*corev1.ServiceAccount, string) {
	GinkgoHelper()

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: f.rootNamespace,
			Name:      name,
		},
	}
	Expect(f.k8sClient.Create(context.Background(), serviceAccount)).To(Succeed())

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

	return serviceAccount, string(serviceAccountSecret.Data[corev1.ServiceAccountTokenKey])
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

func (f *ServiceAccountFactory) FullyQualifiedName(svcAcctName string) string {
	return fmt.Sprintf("system:serviceaccount:%s:%s", f.rootNamespace, svcAcctName)
}
