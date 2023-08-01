package fail_handler

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"code.cloudfoundry.org/korifi/tools"
	"github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	rest "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PodContainerDescriptor struct {
	Namespace     string
	LabelKey      string
	LabelValue    string
	Container     string
	CorrelationId string
}

func PrintPodsLogs(config *rest.Config, podContainerDescriptors []PodContainerDescriptor) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(ginkgo.GinkgoWriter, "failed to create clientset: %v\n", err)
		return
	}

	for _, desc := range podContainerDescriptors {
		pods, err := getPods(clientset, desc.Namespace, desc.LabelKey, desc.LabelValue)
		if err != nil {
			fmt.Fprintf(ginkgo.GinkgoWriter, "Failed to get pods with label %s=%s: %v\n", desc.LabelKey, desc.LabelValue, err)
			continue
		}

		if len(pods) == 0 {
			fmt.Fprintf(ginkgo.GinkgoWriter, "No pods with label %s=%s found\n", desc.LabelKey, desc.LabelValue)
			continue
		}

		for _, pod := range pods {
			for _, container := range selectContainers(pod, desc.Container) {
				printPodContainerLogs(clientset, pod, container, desc.CorrelationId)
			}
		}
	}
}

func PrintPodEvents(config *rest.Config, podContainerDescriptors []PodContainerDescriptor) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(ginkgo.GinkgoWriter, "failed to create clientset: %v\n", err)
		return
	}

	for _, desc := range podContainerDescriptors {
		pods, err := getPods(clientset, desc.Namespace, desc.LabelKey, desc.LabelValue)
		if err != nil {
			fmt.Fprintf(ginkgo.GinkgoWriter, "Failed to get pods with label %s=%s: %v\n", desc.LabelKey, desc.LabelValue, err)
			continue
		}

		if len(pods) == 0 {
			fmt.Fprintf(ginkgo.GinkgoWriter, "No pods with label %s=%s found\n", desc.LabelKey, desc.LabelValue)
			continue
		}

		for _, pod := range pods {
			printEvents(clientset, &pod)
		}
	}
}

func printEvents(clientset kubernetes.Interface, obj client.Object) {
	fmt.Fprintf(ginkgo.GinkgoWriter, "\n========== Events for %s %s/%s ==========\n",
		obj.GetObjectKind().GroupVersionKind().Kind, obj.GetNamespace(), obj.GetName())
	events, err := clientset.CoreV1().Events(obj.GetNamespace()).List(context.Background(), metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + obj.GetName(),
	})
	if err != nil {
		fmt.Fprintf(ginkgo.GinkgoWriter, "Failed to get events: %v", err)
		return
	}

	fmt.Fprint(ginkgo.GinkgoWriter, "LAST SEEN\tTYPE\tREASON\tMESSAGE\n")
	for _, event := range events.Items {
		fmt.Fprintf(ginkgo.GinkgoWriter, "%s\t%s\t%s\t%s\n", event.LastTimestamp, event.Type, event.Reason, event.Message)
	}
}

func getPods(clientset kubernetes.Interface, namespace, labelKey, labelValue string) ([]corev1.Pod, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", labelKey, labelValue),
	})
	if err != nil {
		return nil, err
	}

	return pods.Items, nil
}

func selectContainers(pod corev1.Pod, container string) []string {
	if container != "" {
		return []string{container}
	}

	result := []string{}
	for _, initC := range pod.Spec.InitContainers {
		result = append(result, initC.Name)
	}
	for _, c := range pod.Spec.Containers {
		result = append(result, c.Name)
	}

	return result
}

func printPodContainerLogs(clientset kubernetes.Interface, pod corev1.Pod, container, correlationId string) {
	log, err := getPodContainerLog(clientset, pod, container, correlationId)
	if err != nil {
		fmt.Fprintf(ginkgo.GinkgoWriter, "Failed to get logs for pod %q, container %q: %v\n", pod.Name, container, err)
		return

	}
	if log == "" {
		log = "No relevant logs found"
	}

	logHeader := fmt.Sprintf(
		"Logs for pod %q, container %q",
		pod.Name,
		container,
	)
	if !fullLogOnErr() && correlationId != "" {
		logHeader = fmt.Sprintf(
			"Logs for pod %q, container %q with correlation ID %q",
			pod.Name,
			container,
			correlationId,
		)
	}

	fmt.Fprintf(ginkgo.GinkgoWriter,
		"\n\n===== %s =====\n%s\n==============================================\n\n",
		logHeader,
		log)
}

func getPodContainerLog(clientset kubernetes.Interface, pod corev1.Pod, container, correlationId string) (string, error) {
	podLogOpts := corev1.PodLogOptions{
		SinceTime: tools.PtrTo(metav1.NewTime(ginkgo.CurrentSpecReport().StartTime)),
		Container: container,
	}
	req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &podLogOpts)

	logStream, err := req.Stream(context.Background())
	if err != nil {
		return "", err
	}
	defer logStream.Close()

	var logBuf bytes.Buffer
	logScanner := bufio.NewScanner(logStream)

	for logScanner.Scan() {
		if fullLogOnErr() || strings.Contains(logScanner.Text(), correlationId) {
			logBuf.WriteString(logScanner.Text() + "\n")
		}
	}

	return logBuf.String(), logScanner.Err()
}

func fullLogOnErr() bool {
	return os.Getenv("FULL_LOG_ON_ERR") != ""
}
