package fail_handler

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/tools"
	"github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	rest "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PodDescriptor struct {
	Namespace     string
	LabelKey      string
	LabelValue    string
	CorrelationId string
	Since         *metav1.Time
}

func PrintPodsLogs(config *rest.Config, podDescriptors []PodDescriptor) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(ginkgo.GinkgoWriter, "failed to create clientset: %v\n", err)
		return
	}

	for _, desc := range podDescriptors {
		pods, err := getPods(clientset, desc)
		if err != nil {
			fmt.Fprintf(ginkgo.GinkgoWriter, "Failed to get pods with label %s=%s: %v\n", desc.LabelKey, desc.LabelValue, err)
			continue
		}

		if len(pods) == 0 {
			fmt.Fprintf(ginkgo.GinkgoWriter, "No pods with label %s=%s found\n", desc.LabelKey, desc.LabelValue)
			continue
		}

		for _, pod := range pods {
			printPodLogs(clientset, pod, desc)
		}
	}
}

func PrintPodEvents(config *rest.Config, podDescriptors []PodDescriptor) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(ginkgo.GinkgoWriter, "failed to create clientset: %v\n", err)
		return
	}

	for _, desc := range podDescriptors {
		pods, err := getPods(clientset, desc)
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

func getPods(clientset kubernetes.Interface, desc PodDescriptor) ([]corev1.Pod, error) {
	labelSelector := fmt.Sprintf("%s=%s", desc.LabelKey, desc.LabelValue)
	if desc.LabelValue == "" {
		labelSelector = desc.LabelKey
	}

	pods, err := clientset.CoreV1().Pods(desc.Namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}

	return pods.Items, nil
}

func podContainers(pod corev1.Pod) []string {
	result := []string{}
	for _, initC := range pod.Spec.InitContainers {
		result = append(result, initC.Name)
	}
	for _, c := range pod.Spec.Containers {
		result = append(result, c.Name)
	}

	return result
}

func printPodLogs(clientset kubernetes.Interface, pod corev1.Pod, desc PodDescriptor) {
	for _, container := range podContainers(pod) {
		log, err := getContainerLog(clientset, pod, desc, container)
		if err != nil {
			fmt.Fprintf(ginkgo.GinkgoWriter, "Failed to get logs for pod %q: %v\n", pod.Name, err)
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
		if !fullLogOnErr() && desc.CorrelationId != "" {
			logHeader = fmt.Sprintf(
				"Logs for pod %q with correlation ID %q, container %q",
				pod.Name,
				desc.CorrelationId,
				container,
			)
		}

		fmt.Fprintf(ginkgo.GinkgoWriter,
			"\n\n===== %s =====\n%s\n==============================================\n\n",
			logHeader,
			log)
	}
}

func getContainerLog(clientset kubernetes.Interface, pod corev1.Pod, desc PodDescriptor, container string) (string, error) {
	podLogOpts := corev1.PodLogOptions{
		SinceTime: desc.Since,
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
		if fullLogOnErr() || strings.Contains(logScanner.Text(), desc.CorrelationId) {
			logBuf.WriteString(logScanner.Text() + "\n")
		}
	}

	return logBuf.String(), logScanner.Err()
}

func fullLogOnErr() bool {
	return os.Getenv("FULL_LOG_ON_ERR") != ""
}

func PrintBuildLogs(config *rest.Config, buildGUID string) {
	fmt.Fprint(ginkgo.GinkgoWriter, "\n\n========== Droplet build logs ==========\n")
	fmt.Fprintf(ginkgo.GinkgoWriter, "DropletGUID: %q\n", buildGUID)
	PrintPodsLogs(config, []PodDescriptor{
		{
			LabelKey:   "korifi.cloudfoundry.org/build-workload-name",
			LabelValue: buildGUID,
		},
	})
	PrintPodEvents(config, []PodDescriptor{
		{
			LabelKey:   "korifi.cloudfoundry.org/build-workload-name",
			LabelValue: buildGUID,
		},
	})
}

func PrintKorifiLogs(config *rest.Config, correlationId string, since time.Time) {
	PrintPodsLogs(config, []PodDescriptor{
		{
			Namespace:     "korifi",
			LabelKey:      "app",
			LabelValue:    "korifi-api",
			CorrelationId: correlationId,
			Since:         tools.PtrTo(metav1.NewTime(since)),
		},
		{
			Namespace:  "korifi",
			LabelKey:   "app",
			LabelValue: "korifi-controllers",
			Since:      tools.PtrTo(metav1.NewTime(since)),
		},
	})
}

func PrintKpackLogs(config *rest.Config, since time.Time) {
	PrintPodsLogs(config, []PodDescriptor{
		{
			Namespace:  "kpack",
			LabelKey:   "app",
			LabelValue: "kpack-controller",
			Since:      tools.PtrTo(metav1.NewTime(since)),
		},
		{
			Namespace:  "kpack",
			LabelKey:   "app",
			LabelValue: "kpack-webhook",
			Since:      tools.PtrTo(metav1.NewTime(since)),
		},
	})
}

func PrintAllBuildLogs(config *rest.Config, namespace string) {
	PrintPodsLogs(config, []PodDescriptor{
		{
			Namespace: namespace,
			LabelKey:  "kpack.io/build",
		},
	})
}
