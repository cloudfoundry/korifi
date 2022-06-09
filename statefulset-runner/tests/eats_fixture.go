package tests

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type EATSFixture struct {
	Fixture

	DynamicClientset dynamic.Interface
}

func NewEATSFixture(baseFixture Fixture, dynamicClientset dynamic.Interface) *EATSFixture {
	return &EATSFixture{
		Fixture:          baseFixture,
		DynamicClientset: dynamicClientset,
	}
}

func (f *EATSFixture) SetUp() {
	f.Fixture.SetUp()
	CopyRolesAndBindings(f.Namespace, f.Fixture.Clientset)
}

func (f *EATSFixture) TearDown() {
	if f == nil {
		Fail("failed to initialize fixture")

		return
	}

	f.Fixture.TearDown()
}

func (f *EATSFixture) GetNATSPassword() string {
	secret, err := f.Clientset.CoreV1().Secrets(GetEiriniSystemNamespace()).Get(context.Background(), "nats-secret", metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	return string(secret.Data["nats-password"])
}

func CopyRolesAndBindings(namespace string, clientset kubernetes.Interface) {
	from := GetEiriniWorkloadsNamespace()

	roleList, err := clientset.RbacV1().Roles(from).List(context.Background(), metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	for _, role := range roleList.Items {
		newRole := new(rbacv1.Role)
		newRole.Namespace = namespace
		newRole.Name = role.Name
		newRole.Rules = role.Rules
		_, err = clientset.RbacV1().Roles(namespace).Create(context.Background(), newRole, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	}

	bindingList, err := clientset.RbacV1().RoleBindings(from).List(context.Background(), metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	for _, binding := range bindingList.Items {
		newBinding := new(rbacv1.RoleBinding)
		newBinding.Namespace = namespace
		newBinding.Name = binding.Name
		newBinding.Subjects = binding.Subjects

		if binding.Name == "eirini-workloads-app-rolebinding" {
			newBinding.Subjects[0].Namespace = namespace
		}

		newBinding.RoleRef = binding.RoleRef
		_, err := clientset.RbacV1().RoleBindings(namespace).Create(context.Background(), newBinding, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}
