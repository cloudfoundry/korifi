package helpers

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	. "github.com/onsi/gomega"

	. "github.com/onsi/ginkgo/v2"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func BindUserToControllersRole(k8sClient client.Client, userName string) {
	GinkgoHelper()

	controllersRole := ensureControllersClusterRole(k8sClient)

	Expect(k8sClient.Create(context.Background(), &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "envtest-controller",
		},
		Subjects: []rbacv1.Subject{{
			Kind: "User",
			Name: userName,
		}},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: controllersRole.Name,
		},
	})).To(Succeed())
}

func ensureControllersClusterRole(k8sClient client.Client) *rbacv1.ClusterRole {
	clusterRoleDefinition, err := os.ReadFile(controllersRoleYamlPath())
	Expect(err).NotTo(HaveOccurred())

	roleObject, _, err := scheme.Codecs.UniversalDeserializer().Decode(clusterRoleDefinition, nil, new(rbacv1.ClusterRole))
	Expect(err).NotTo(HaveOccurred())

	clusterRole, ok := roleObject.(*rbacv1.ClusterRole)
	Expect(ok).To(BeTrue())

	Expect(client.IgnoreAlreadyExists(k8sClient.Create(context.Background(), clusterRole))).To(Succeed())

	return clusterRole
}

func controllersRoleYamlPath() string {
	_, thisFilePath, _, ok := runtime.Caller(0)
	Expect(ok).To(BeTrue())
	thisFileDir := filepath.Dir(thisFilePath)

	return filepath.Join(thisFileDir, "..", "..", "helm", "korifi", "controllers", "role.yaml")
}
