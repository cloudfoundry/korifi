package smoke_test

import (
	"fmt"
	"os/exec"
	"sync"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
)

const BeforeEachThreads = 50

var _ = Describe("apps", func() {
	var experiment *gmeasure.Experiment

	BeforeEach(func() {
		experiment = gmeasure.NewExperiment("List Apps")
		AddReportEntry(experiment.Name, experiment)
	})

	runTest := func(size int) {
		// setup
		wg := sync.WaitGroup{}
		wg.Add(size)

		workQueue := make(chan any, size)
		for i := 0; i < size; i++ {
			workQueue <- struct{}{}
		}

		for i := 0; i < BeforeEachThreads; i++ {
			go func() {
				GinkgoRecover()

				for range workQueue {
					spaceName := uuid.NewString()
					Expect(cf("create-space", "-o", orgName, spaceName).Run()).To(Succeed())
					Expect(cf("target", "-o", orgName, "-s", spaceName).Run()).To(Succeed())
					Expect(cf("create-app", uuid.NewString()).Run()).To(Succeed())
					wg.Done()
				}
			}()
		}

		wg.Wait()
		close(workQueue)

		// test
		experiment.Sample(func(idx int) {
			cmd := cf("curl", "v3/apps")

			var err error
			experiment.MeasureDuration(fmt.Sprintf("list apps (%4d spaces)", size), func() {
				err = cmd.Run()
			})
			Expect(err).NotTo(HaveOccurred())
		}, gmeasure.SamplingConfig{N: 10})
	}

	It("lists app efficiently", func() {
		runTest(1)
		runTest(10)
		runTest(100)
		runTest(1000)
	})
})

func cf(args ...string) *exec.Cmd {
	cmd := exec.Command("cf", args...)
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter

	return cmd
}
