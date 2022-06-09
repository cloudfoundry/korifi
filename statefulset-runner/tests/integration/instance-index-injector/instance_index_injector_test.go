package instance_index_injector_test

import (
	"context"

	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("InstanceIndexInjector", func() {
	var pod *corev1.Pod

	BeforeEach(func() {
		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "app-name-0",
				Labels: map[string]string{
					stset.LabelSourceType: "APP",
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  stset.ApplicationContainerName,
						Image: "eirini/dorini",
					},
					{
						Name:  "not-application",
						Image: "eirini/dorini",
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		var err error
		pod, err = fixture.Clientset.CoreV1().Pods(fixture.Namespace).Create(context.Background(), pod, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	getCFInstanceIndex := func(pod *corev1.Pod, containerName string) string {
		for _, container := range pod.Spec.Containers {
			if container.Name != containerName {
				continue
			}

			for _, e := range container.Env {
				if e.Name != eirinictrl.EnvCFInstanceIndex {
					continue
				}

				return e.Value
			}
		}

		return ""
	}

	It("sets CF_INSTANCE_INDEX in the application container environment", func() {
		Eventually(func() string { return getCFInstanceIndex(pod, stset.ApplicationContainerName) }).Should(Equal("0"))
	})

	It("does not set CF_INSTANCE_INDEX on the non-application container", func() {
		Expect(getCFInstanceIndex(pod, "not-application")).To(Equal(""))
	})
})
