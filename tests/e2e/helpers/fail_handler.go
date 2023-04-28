package helpers

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"code.cloudfoundry.org/korifi/tools"
	"github.com/onsi/ginkgo/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

const (
	logTailLines int64 = 50
)

type podContainerDescriptor struct {
	Namespace     string
	LabelKey      string
	LabelValue    string
	Container     string
	CorrelationId string
}

func E2EFailHandler(correlationId func() string) func(string, ...int) {
	return func(message string, callerSkip ...int) {
		fmt.Fprintf(ginkgo.GinkgoWriter, "Fail Handler: failure correlation ID: %q\n", correlationId())

		if len(callerSkip) > 0 {
			callerSkip[0] = callerSkip[0] + 1
		} else {
			callerSkip = []int{1}
		}

		defer func() {
			fmt.Fprintln(ginkgo.GinkgoWriter, "Fail Handler: completed")
			ginkgo.Fail(message, callerSkip...)
		}()

		config, err := controllerruntime.GetConfig()
		if err != nil {
			fmt.Fprintf(ginkgo.GinkgoWriter, "failed to get kubernetes config: %v\n", err)
			return
		}

		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			fmt.Fprintf(ginkgo.GinkgoWriter, "failed to create clientset: %v\n", err)
			return
		}

		namespace := systemNamespace()
		printPodsLogs(clientset, []podContainerDescriptor{
			{
				Namespace:     namespace,
				LabelKey:      "app",
				LabelValue:    "korifi-api",
				Container:     "korifi-api",
				CorrelationId: correlationId(),
			},
			{
				Namespace:  namespace,
				LabelKey:   "app",
				LabelValue: "korifi-controllers",
				Container:  "manager",
			},
		})

		if strings.Contains(message, "Droplet not found") {
			printDropletNotFoundDebugInfo(clientset, message)
		}

		if strings.Contains(message, "404") {
			printAllRoleBindings(clientset)
		}
	}
}

func systemNamespace() string {
	systemNS, found := os.LookupEnv("SYSTEM_NAMESPACE")
	if found {
		return systemNS
	}

	return "korifi"
}

func fullLogOnErr() bool {
	return os.Getenv("FULL_LOG_ON_ERR") != ""
}

func printPodsLogs(clientset kubernetes.Interface, podContainerDescriptors []podContainerDescriptor) {
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
		"Logs for pod %q, container %q (last %d lines)",
		pod.Name,
		container,
		logTailLines,
	)
	if !fullLogOnErr() && correlationId != "" {
		logHeader = fmt.Sprintf(
			"Logs for pod %q, container %q with correlation ID %q (last %d lines)",
			pod.Name,
			container,
			correlationId,
			logTailLines,
		)
	}

	fmt.Fprintf(ginkgo.GinkgoWriter,
		"\n\n===== %s =====\n%s\n==============================================\n\n",
		logHeader,
		log)
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

func getPodContainerLog(clientset kubernetes.Interface, pod corev1.Pod, container, correlationId string) (string, error) {
	podLogOpts := corev1.PodLogOptions{TailLines: tools.PtrTo(logTailLines), Container: container}
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

func printDropletNotFoundDebugInfo(clientset kubernetes.Interface, message string) {
	fmt.Fprint(ginkgo.GinkgoWriter, "\n\n========== Droplet not found debug log (start) ==========\n")

	fmt.Fprint(ginkgo.GinkgoWriter, "\n========== Kpack logs ==========\n")
	printPodsLogs(clientset, []podContainerDescriptor{
		{
			Namespace:  "kpack",
			LabelKey:   "app",
			LabelValue: "kpack-controller",
		},
		{
			Namespace:  "kpack",
			LabelKey:   "app",
			LabelValue: "kpack-webhook",
		},
	})

	dropletGUID, err := getDropletGUID(message)
	if err != nil {
		fmt.Fprintf(ginkgo.GinkgoWriter, "Failed to get droplet GUID from message %v\n", err)
		return
	}

	fmt.Fprint(ginkgo.GinkgoWriter, "\n\n========== Droplet build logs ==========\n")
	fmt.Fprintf(ginkgo.GinkgoWriter, "DropletGUID: %q\n", dropletGUID)
	printPodsLogs(clientset, []podContainerDescriptor{
		{
			LabelKey:   "korifi.cloudfoundry.org/build-workload-name",
			LabelValue: dropletGUID,
		},
	})

	fmt.Fprint(ginkgo.GinkgoWriter, "\n\n========== Droplet not found debug log (end) ==========\n\n")
}

func getDropletGUID(message string) (string, error) {
	r := regexp.MustCompile(`Request.*droplets/(.*)`)
	matches := r.FindStringSubmatch(message)
	if len(matches) != 2 {
		return "", fmt.Errorf("message does not match regex: %s", r.String())
	}

	return matches[1], nil
}

func printAllRoleBindings(clientset kubernetes.Interface) {
	list, err := clientset.RbacV1().RoleBindings("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("failed getting rolebindings: %v", err)
		return
	}

	fmt.Fprint(ginkgo.GinkgoWriter, "\n\n========== Expected 404 debug log ==========\n\n")
	for _, b := range list.Items {
		fmt.Fprintf(ginkgo.GinkgoWriter, "Name: %s, Namespace: %s, RoleKind: %s, RoleName: %s, Subjects: \n",
			b.Name, b.Namespace, b.RoleRef.Kind, b.RoleRef.Name)
		for _, s := range b.Subjects {
			fmt.Fprintf(ginkgo.GinkgoWriter, "\tKind: %s, Name: %s, Namespace: %s\n", s.Kind, s.Name, s.Namespace)
		}
	}
	fmt.Fprint(ginkgo.GinkgoWriter, "\n\n========== Expected 404 debug log (end) ==========\n\n")
}
