package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/jobs"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	eiriniclient "code.cloudfoundry.org/korifi/statefulset-runner/pkg/generated/clientset/versioned"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	"code.cloudfoundry.org/tlsconfig"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/ghttp"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func ListJobs(clientset kubernetes.Interface, namespace, taskGUID string) func() []batchv1.Job {
	return func() []batchv1.Job {
		jobs, err := clientset.BatchV1().
			Jobs(namespace).
			List(context.Background(), metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", jobs.LabelGUID, taskGUID)})

		Expect(err).NotTo(HaveOccurred())

		return jobs.Items
	}
}

func GetTaskJobConditions(clientset kubernetes.Interface, namespace, taskGUID string) func() []batchv1.JobCondition {
	return func() []batchv1.JobCondition {
		jobs := ListJobs(clientset, namespace, taskGUID)()

		return jobs[0].Status.Conditions
	}
}

func GetRegistrySecretName(clientset kubernetes.Interface, namespace, taskGUID, secretName string) string {
	jobs := ListJobs(clientset, namespace, taskGUID)()
	imagePullSecrets := jobs[0].Spec.Template.Spec.ImagePullSecrets

	var registrySecretName string

	for _, imagePullSecret := range imagePullSecrets {
		if strings.HasPrefix(imagePullSecret.Name, secretName) {
			registrySecretName = imagePullSecret.Name
		}
	}

	Expect(registrySecretName).NotTo(BeEmpty())

	return registrySecretName
}

func CreateEmptySecret(namespace, secretName string, clientset kubernetes.Interface) error {
	_, err := clientset.CoreV1().Secrets(namespace).Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
	}, metav1.CreateOptions{})

	return err
}

func CreateSecretWithStringData(namespace, secretName string, clientset kubernetes.Interface, stringData map[string]string) error {
	_, err := clientset.CoreV1().Secrets(namespace).Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		StringData: stringData,
	}, metav1.CreateOptions{})

	return err
}

func MakeTestHTTPClient(certsPath string) (*http.Client, error) {
	bs, err := ioutil.ReadFile(filepath.Join(certsPath, "tls.ca"))
	if err != nil {
		return nil, err
	}

	clientCert, err := tls.LoadX509KeyPair(filepath.Join(certsPath, "tls.crt"), filepath.Join(certsPath, "tls.key"))
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(bs) {
		return nil, err
	}

	return newTlsClient(&tls.Config{
		MinVersion:   tls.VersionTLS12,
		RootCAs:      certPool,
		Certificates: []tls.Certificate{clientCert},
	}), nil
}

func newTlsClient(tlsConfig *tls.Config) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
			IdleConnTimeout: 90 * time.Second,
			TLSClientConfig: tlsConfig,
		},
	}
}

func DefaultControllerConfig(namespace string) *eirinictrl.ControllerConfig {
	return &eirinictrl.ControllerConfig{
		KubeConfig: eirinictrl.KubeConfig{
			ConfigPath: tests.GetKubeconfig(),
		},
		ApplicationServiceAccount: tests.GetApplicationServiceAccount(),
		RegistrySecretName:        "registry-secret",
		WorkloadsNamespace:        namespace,
		WebhookPort:               int32(8080 + ginkgo.GinkgoParallelProcess()),
		TaskTTLSeconds:            5,
		LeaderElectionID:          fmt.Sprintf("test-eirini-%d", ginkgo.GinkgoParallelProcess()),
		LeaderElectionNamespace:   namespace,
	}
}

func CreateConfigFile(config interface{}) (*os.File, error) {
	yamlBytes, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}

	configFile, err := ioutil.TempFile("", "config.yml")
	if err != nil {
		return nil, err
	}

	err = ioutil.WriteFile(configFile.Name(), yamlBytes, os.ModePerm)

	return configFile, err
}

func CreateTestServer(certPath, keyPath, caCertPath string) (*ghttp.Server, error) {
	tlsConf, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentityFromFile(certPath, keyPath),
	).Server(
		tlsconfig.WithClientAuthenticationFromFile(caCertPath),
	)
	if err != nil {
		return nil, err
	}

	testServer := ghttp.NewUnstartedServer()
	testServer.HTTPTestServer.TLS = tlsConf

	return testServer, nil
}

func GetPDBItems(clientset kubernetes.Interface, namespace, lrpGUID, lrpVersion string) ([]policyv1.PodDisruptionBudget, error) {
	pdbList, err := clientset.PolicyV1beta1().PodDisruptionBudgets(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s,%s=%s", stset.LabelGUID, lrpGUID, stset.LabelVersion, lrpVersion),
	})
	if err != nil {
		return nil, err
	}

	return pdbList.Items, nil
}

func GetPDB(clientset kubernetes.Interface, namespace, lrpGUID, lrpVersion string) policyv1.PodDisruptionBudget {
	var pdbs []policyv1.PodDisruptionBudget

	Eventually(func() ([]policyv1.PodDisruptionBudget, error) {
		var err error
		pdbs, err = GetPDBItems(clientset, namespace, lrpGUID, lrpVersion)

		return pdbs, err
	}).Should(HaveLen(1))

	Consistently(func() ([]policyv1.PodDisruptionBudget, error) {
		var err error
		pdbs, err = GetPDBItems(clientset, namespace, lrpGUID, lrpVersion)

		return pdbs, err
	}, "5s").Should(HaveLen(1))

	return pdbs[0]
}

func GetStatefulSet(clientset kubernetes.Interface, namespace, guid, version string) *appsv1.StatefulSet {
	appListOpts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s,%s=%s", stset.LabelGUID, guid, stset.LabelVersion, version),
	}

	stsList, err := clientset.
		AppsV1().
		StatefulSets(namespace).
		List(context.Background(), appListOpts)

	Expect(err).NotTo(HaveOccurred())

	if len(stsList.Items) == 0 {
		return nil
	}

	Expect(stsList.Items).To(HaveLen(1))

	return &stsList.Items[0]
}

func GetLRP(clientset eiriniclient.Interface, namespace, lrpName string) *eiriniv1.LRP {
	l, err := clientset.
		EiriniV1().
		LRPs(namespace).
		Get(context.Background(), lrpName, metav1.GetOptions{})

	Expect(err).NotTo(HaveOccurred())

	return l
}

func GetTaskExecutionStatus(clientset eiriniclient.Interface, namespace, taskName string) func() eiriniv1.ExecutionStatus {
	return func() eiriniv1.ExecutionStatus {
		task, err := clientset.
			EiriniV1().
			Tasks(namespace).
			Get(context.Background(), taskName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		return task.Status.ExecutionStatus
	}
}
