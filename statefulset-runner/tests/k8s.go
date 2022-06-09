package tests

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
)

const DefaultApplicationServiceAccount = "eirini"

func CreateRandomNamespace(clientset kubernetes.Interface) string {
	namespace := fmt.Sprintf("integration-test-%s-%d", GenerateGUID(), GinkgoParallelProcess())
	for namespaceExists(namespace, clientset) {
		namespace = fmt.Sprintf("integration-test-%s-%d", GenerateGUID(), GinkgoParallelProcess())
	}
	createNamespace(namespace, clientset)

	return namespace
}

func namespaceExists(namespace string, clientset kubernetes.Interface) bool {
	_, err := clientset.CoreV1().Namespaces().Get(context.Background(), namespace, metav1.GetOptions{})

	return err == nil
}

func createNamespace(namespace string, clientset kubernetes.Interface) {
	namespaceSpec := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}

	_, err := clientset.CoreV1().Namespaces().Create(context.Background(), namespaceSpec, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func CreatePodCreationPSP(namespace, pspName, serviceAccountName string, clientset kubernetes.Interface) error {
	_, err := clientset.PolicyV1beta1().PodSecurityPolicies().Create(context.Background(), &policyv1.PodSecurityPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: pspName,
			Annotations: map[string]string{
				"seccomp.security.alpha.kubernetes.io/allowedProfileNames": "runtime/default",
				"seccomp.security.alpha.kubernetes.io/defaultProfileName":  "runtime/default",
			},
		},
		Spec: policyv1.PodSecurityPolicySpec{
			Privileged: false,
			RunAsUser: policyv1.RunAsUserStrategyOptions{
				Rule: policyv1.RunAsUserStrategyRunAsAny,
			},
			SELinux: policyv1.SELinuxStrategyOptions{
				Rule: policyv1.SELinuxStrategyRunAsAny,
			},
			SupplementalGroups: policyv1.SupplementalGroupsStrategyOptions{
				Rule: policyv1.SupplementalGroupsStrategyRunAsAny,
			},
			FSGroup: policyv1.FSGroupStrategyOptions{
				Rule: policyv1.FSGroupStrategyRunAsAny,
			},
			Volumes: []policyv1.FSType{
				policyv1.EmptyDir, policyv1.Projected, policyv1.Secret,
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	roleName := "use-psp"
	_, err = clientset.RbacV1().Roles(namespace).Create(context.Background(), &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleName,
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{"policy"},
				Resources:     []string{"podsecuritypolicies"},
				Verbs:         []string{"use"},
				ResourceNames: []string{pspName},
			},
		},
	}, metav1.CreateOptions{})

	if err != nil {
		return err
	}

	_, err = clientset.CoreV1().ServiceAccounts(namespace).Create(context.Background(), &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountName,
			Namespace: namespace,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	_, err = clientset.RbacV1().RoleBindings(namespace).Create(context.Background(), &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-account-psp",
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      serviceAccountName,
			Namespace: namespace,
		}},
	}, metav1.CreateOptions{})

	return err
}

func DeleteNamespace(namespace string, clientset kubernetes.Interface) error {
	return clientset.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
}

func DeletePSP(name string, clientset kubernetes.Interface) error {
	return clientset.PolicyV1beta1().PodSecurityPolicies().Delete(context.Background(), name, metav1.DeleteOptions{})
}

func GetApplicationServiceAccount() string {
	serviceAccountName := os.Getenv("APPLICATION_SERVICE_ACCOUNT")
	if serviceAccountName != "" {
		return serviceAccountName
	}

	return DefaultApplicationServiceAccount
}

func ExposeAsService(clientset kubernetes.Interface, namespace, guid string, appPort int32, pingPath ...string) string {
	service, err := clientset.CoreV1().Services(namespace).Create(context.Background(), &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: "service-" + guid,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port: appPort,
				},
			},
			Selector: map[string]string{
				stset.LabelGUID: guid,
			},
		},
	}, metav1.CreateOptions{})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	if len(pingPath) > 0 {
		EventuallyWithOffset(1, func() error {
			_, err := RequestServiceFn(namespace, service.Name, appPort, pingPath[0])()

			return err
		}).Should(Succeed())
	}

	return service.Name
}

func RequestServiceFn(namespace, serviceName string, port int32, requestPath string) func() (string, error) {
	client := &http.Client{
		Timeout: time.Second,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
	}

	return func() (_ string, err error) {
		defer func() {
			if err != nil {
				fmt.Fprintf(GinkgoWriter, "RequestServiceFn error: %v", err)
			}
		}()

		requestURL := fmt.Sprintf("http://%s.%s:%d/%s", serviceName, namespace, port, requestPath)

		resp, err := client.Get(requestURL)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		content, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		if resp.StatusCode != http.StatusOK {
			return string(content), fmt.Errorf("request failed: %s", resp.Status)
		}

		return string(content), nil
	}
}
