package eats_test

import (
	"context"
	"fmt"
	"time"

	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("PodCrashEvents [needs-logs-for: eirini-api, eirini-controller]", func() {
	var (
		lrpName    string
		lrpGUID    string
		lrpVersion string
		timestamp  time.Time
	)

	BeforeEach(func() {
		timestamp = time.Now()
	})

	When("a crashing app is deployed", func() {
		BeforeEach(func() {
			namespace := fixture.Namespace
			lrpName = tests.GenerateGUID()
			lrpGUID = tests.GenerateGUID()
			lrpVersion = tests.GenerateGUID()

			lrp := &eiriniv1.LRP{
				ObjectMeta: metav1.ObjectMeta{
					Name: lrpName,
				},
				Spec: eiriniv1.LRPSpec{
					GUID:      lrpGUID,
					Version:   lrpVersion,
					Image:     "eirini/busybox",
					AppGUID:   "the-app-guid",
					AppName:   "k-2so",
					SpaceName: "s",
					OrgName:   "o",
					MemoryMB:  256,
					DiskMB:    256,
					CPUWeight: 10,
					Instances: 1,
					Command:   []string{"sh", "-c", `sleep 1; exit 3`},
				},
			}

			_, err := fixture.EiriniClientset.
				EiriniV1().
				LRPs(namespace).
				Create(context.Background(), lrp, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("creates crash events", func() {
			eventsClient := fixture.Clientset.CoreV1().Events(fixture.Namespace)
			getEvents := func() []corev1.Event {
				eventList, err := eventsClient.List(context.Background(), metav1.ListOptions{
					FieldSelector: "involvedObject.kind=LRP",
				})
				Expect(err).NotTo(HaveOccurred())

				return eventList.Items
			}
			Eventually(getEvents).Should(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Reason": Equal("Container: Error"),
			})))
		})

		It("populates the events with relevant information", func() {
			eventsClient := fixture.Clientset.CoreV1().Events(fixture.Namespace)
			var events []corev1.Event
			getEvents := func() int {
				eventList, err := eventsClient.List(context.Background(), metav1.ListOptions{
					FieldSelector: fmt.Sprintf("involvedObject.kind=LRP,involvedObject.name=%s", lrpName),
				})
				Expect(err).NotTo(HaveOccurred())
				events = eventList.Items

				return len(events)
			}
			Eventually(getEvents).Should(BeNumerically(">", 0))

			crash := events[0]
			Expect(crash.Name).To(HavePrefix("k-2so"))
			Expect(crash.Type).To(Equal("Warning"))
			Expect(crash.Count).To(BeNumerically("==", 1))
			Expect(crash.FirstTimestamp.Time).To(BeTemporally(">", timestamp))
			Expect(crash.LastTimestamp.Time).To(BeTemporally("==", crash.FirstTimestamp.Time))
			Expect(crash.EventTime.Time).To(BeTemporally(">", crash.LastTimestamp.Time))
			Expect(crash.Message).To(Equal("Container terminated with exit code: 3"))
			Expect(crash.Source.Component).To(Equal("eirini-controller"))
			Expect(crash.Labels).To(HaveKeyWithValue("korifi.cloudfoundry.org/instance-index", "0"))
			Expect(crash.Annotations).To(HaveKeyWithValue("korifi.cloudfoundry.org/process-guid", fmt.Sprintf("%s-%s", lrpGUID, lrpVersion)))
			Expect(crash.InvolvedObject.Kind).To(Equal("LRP"))
			Expect(crash.InvolvedObject.Name).To(Equal(lrpName))
			Expect(crash.InvolvedObject.Namespace).To(Equal(fixture.Namespace))
		})

		It("updates crash events", func() {
			eventsClient := fixture.Clientset.CoreV1().Events(fixture.Namespace)
			getEvents := func() []corev1.Event {
				eventList, err := eventsClient.List(context.Background(), metav1.ListOptions{
					FieldSelector: "involvedObject.kind=LRP",
				})
				Expect(err).NotTo(HaveOccurred())

				return eventList.Items
			}
			Eventually(getEvents).Should(ContainElement(MatchFields(IgnoreExtras, Fields{
				"Reason": HavePrefix("Container:"),
				"Count":  BeNumerically(">", 1),
			})))
		})
	})
})
