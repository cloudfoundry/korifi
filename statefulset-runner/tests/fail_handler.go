package tests

import (
	"context"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	logTailLines      = 300
	describeLogsRegEx = ".*\\[needs-logs-for:(.*)\\].*"
)

func EatsFailHandler(message string, callerSkip ...int) {
	fixture := NewFixture(nil, GinkgoWriter)
	printPodsLogs(fixture.Clientset, getRelatedEiriniPodsNames(CurrentGinkgoTestDescription().FullTestText))

	Fail(message, callerSkip...)
}

func printPodsLogs(clientset kubernetes.Interface, eiriniPodsNames []string) {
	if len(eiriniPodsNames) == 0 {
		fmt.Fprintln(GinkgoWriter, "No Eirini logs have been requested by the test on failure")

		return
	}

	for _, podName := range eiriniPodsNames {
		pods, err := getEiriniPods(clientset, podName)
		if err != nil {
			fmt.Fprintf(GinkgoWriter, "Failed to get pod %s: %v", podName, err)
		}

		for _, pod := range pods {
			log, err := getSinglePodLog(clientset, pod.Name)
			if err != nil {
				fmt.Fprintf(GinkgoWriter, "Failed to get logs for pod %s: %v", pod.Name, err)

				continue
			}

			fmt.Fprintf(GinkgoWriter,
				"\n\n===== Logs for pod %q (last %d lines) =====\n%s\n==============================================\n\n",
				pod.Name,
				logTailLines,
				log)
		}
	}
}

func getRelatedEiriniPodsNames(testText string) []string {
	regEx := regexp.MustCompile(describeLogsRegEx)

	match := regEx.FindStringSubmatch(testText)
	if len(match) < 2 {
		// no Eirini logs have been requested by the test
		return []string{}
	}

	eiriniPods := []string{}
	for _, requestedPod := range strings.Split(strings.TrimSpace(match[1]), ",") {
		eiriniPods = append(eiriniPods, strings.TrimSpace(requestedPod))
	}

	return eiriniPods
}

func getEiriniPods(clientset kubernetes.Interface, podName string) ([]corev1.Pod, error) {
	pods, err := clientset.CoreV1().Pods(GetEiriniSystemNamespace()).List(context.Background(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("name=%s", podName),
	})
	if err != nil {
		return nil, err
	}

	return pods.Items, nil
}

func getSinglePodLog(clientset kubernetes.Interface, podName string) (string, error) {
	podLogOpts := corev1.PodLogOptions{TailLines: int64ptr(logTailLines)}
	req := clientset.CoreV1().Pods(GetEiriniSystemNamespace()).GetLogs(podName, &podLogOpts)

	logStream, err := req.Stream(context.Background())
	if err != nil {
		return "", nil //nolint:nilerr
	}
	defer logStream.Close()

	logBytes, err := ioutil.ReadAll(logStream)
	if err != nil {
		return "", err
	}

	return string(logBytes), nil
}

func int64ptr(i int64) *int64 {
	return &i
}
