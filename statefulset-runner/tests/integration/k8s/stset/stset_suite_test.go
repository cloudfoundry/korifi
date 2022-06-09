package stset_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/pdb"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	eirinischeme "code.cloudfoundry.org/korifi/statefulset-runner/pkg/generated/clientset/versioned/scheme"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policy_v1beta1_types "k8s.io/client-go/kubernetes/typed/policy/v1beta1"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

func TestEiriniK8sClient(t *testing.T) {
	SetDefaultEventuallyTimeout(4 * time.Minute)
	RegisterFailHandler(Fail)
	RunSpecs(t, "StatefulSet Suite")
}

var (
	fixture *tests.Fixture
	ctx     context.Context
)

var _ = BeforeSuite(func() {
	fixture = tests.NewFixture(GinkgoWriter)
})

var _ = BeforeEach(func() {
	fixture.SetUp()
	ctx = context.Background()
})

var _ = AfterEach(func() {
	fixture.TearDown()
})

var _ = AfterSuite(func() {
	fixture.Destroy()
})

func createDesirer(workloadsNamespace string, allowRunImageAsRoot bool) *stset.Desirer {
	logger := tests.NewTestLogger("test-" + workloadsNamespace)

	lrpToStatefulSetConverter := stset.NewLRPToStatefulSetConverter(
		tests.GetApplicationServiceAccount(),
		"registry-secret",
		false,
		allowRunImageAsRoot,
		k8s.CreateLivenessProbe,
		k8s.CreateReadinessProbe,
	)

	pdbUpdater := pdb.NewUpdater(fixture.RuntimeClient)

	return stset.NewDesirer(logger, lrpToStatefulSetConverter, pdbUpdater, fixture.RuntimeClient, eirinischeme.Scheme)
}

func labelSelector(lrp *eiriniv1.LRP) string {
	return fmt.Sprintf(
		"%s=%s,%s=%s",
		stset.LabelGUID, lrp.Spec.GUID,
		stset.LabelVersion, lrp.Spec.Version,
	)
}

func listPodsByLabel(labelSelector string) []corev1.Pod {
	pods, err := fixture.Clientset.CoreV1().Pods(fixture.Namespace).List(context.Background(), metav1.ListOptions{LabelSelector: labelSelector})
	Expect(err).NotTo(HaveOccurred())

	return pods.Items
}

func listPods(lrp *eiriniv1.LRP) []corev1.Pod {
	return listPodsByLabel(labelSelector(lrp))
}

func podDisruptionBudgets() policy_v1beta1_types.PodDisruptionBudgetInterface {
	return fixture.Clientset.PolicyV1beta1().PodDisruptionBudgets(fixture.Namespace)
}

func podNamesFromPods(pods []corev1.Pod) []string {
	names := []string{}
	for _, p := range pods {
		names = append(names, p.Name)
	}

	return names
}

func nodeNamesFromPods(pods []corev1.Pod) []string {
	names := []string{}

	for _, p := range pods {
		nodeName := p.Spec.NodeName
		if nodeName != "" {
			names = append(names, nodeName)
		}
	}

	return names
}

func getNodeCount() int {
	nodeList, err := fixture.Clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	Expect(err).ToNot(HaveOccurred())

	return len(nodeList.Items)
}

func getSecret(ns, name string) (*corev1.Secret, error) {
	return fixture.Clientset.CoreV1().Secrets(ns).Get(context.Background(), name, metav1.GetOptions{})
}

func createLRP(namespace, name string) *eiriniv1.LRP {
	lrp := &eiriniv1.LRP{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: eiriniv1.LRPSpec{
			GUID:    tests.GenerateGUID(),
			Version: tests.GenerateGUID(),
			Command: []string{
				"/bin/sh",
				"-c",
				"while true; do echo hello; sleep 10;done",
			},
			AppName:   name,
			AppGUID:   "the-app-guid",
			SpaceName: "space-foo",
			Instances: 2,
			Image:     "eirini/busybox",
			DiskMB:    2047,
			Env: map[string]string{
				"FOO": "BAR",
			},
		},
	}

	lrp, err := fixture.EiriniClientset.EiriniV1().LRPs(fixture.Namespace).Create(context.Background(), lrp, metav1.CreateOptions{})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	return lrp
}

func int32ptr(i int) *int32 {
	i32 := int32(i)

	return &i32
}

func getPodPhase(index int, lrp *eiriniv1.LRP) string {
	pod := listPods(lrp)[index]
	status := pod.Status

	if status.Phase != corev1.PodRunning {
		return fmt.Sprintf("Pod - %s", status.Phase)
	}

	if len(status.ContainerStatuses) == 0 {
		return "Containers status unknown"
	}

	for _, containerStatus := range status.ContainerStatuses {
		if containerStatus.State.Running == nil {
			return fmt.Sprintf("Container %s - %v", containerStatus.Name, containerStatus.State)
		}

		if !containerStatus.Ready {
			return fmt.Sprintf("Container %s is not Ready", containerStatus.Name)
		}
	}

	return "Ready"
}

func getStatefulSetForLRP(lrp *eiriniv1.LRP) *appsv1.StatefulSet {
	ss, getErr := fixture.Clientset.AppsV1().StatefulSets(fixture.Namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labelSelector(lrp),
	})
	Expect(getErr).NotTo(HaveOccurred())
	Expect(ss.Items).To(HaveLen(1))

	return &ss.Items[0]
}
