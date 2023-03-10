package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("WatchForConfigChangeEvents", func() {
	var (
		configPath string
		eventChan  chan string
		cancelFunc context.CancelFunc
	)

	BeforeEach(func() {
		var err error
		configPath, err = os.MkdirTemp("", "config")
		Expect(err).NotTo(HaveOccurred())

		eventChan = make(chan string)
	})

	JustBeforeEach(func() {
		var subctx context.Context
		subctx, cancelFunc = context.WithCancel(context.Background())

		go func() {
			Expect(tools.WatchForConfigChangeEvents(subctx, configPath, GinkgoLogr, eventChan)).To(Succeed())
		}()

		Eventually(eventChan).WithTimeout(time.Second * 2).Should(Receive(Equal(configPath)))
	})

	AfterEach(func() {
		cancelFunc()
		Expect(os.RemoveAll(configPath)).To(Succeed())
	})

	When("a file is created", func() {
		It("only signals the event channel once", func() {
			_ = writeTempFile(configPath)

			Eventually(eventChan).WithTimeout(time.Second * 2).Should(Receive(Equal(configPath)))
			Consistently(eventChan).WithTimeout(time.Second * 2).ShouldNot(Receive())
		})
	})

	// When("a file is updated", func() {
	// 	var fileName string

	// 	BeforeEach(func() {
	// 		fileName = writeTempFile(configPath)
	// 	})

	// 	It("only signals the event channel once", func() {
	// 		Expect(os.WriteFile(fileName, []byte("some update"), 0o644)).To(Succeed())

	// 		Eventually(eventChan).WithTimeout(time.Second * 2).Should(Receive(Equal(configPath)))
	// 		Consistently(eventChan).WithTimeout(time.Second * 2).ShouldNot(Receive())
	// 	})
	// })

	When("a file is removed", func() {
		var fileName string

		BeforeEach(func() {
			fileName = writeTempFile(configPath)
		})

		It("does not signal the event channel", func() {
			Expect(os.Remove(fileName)).To(Succeed())

			Consistently(eventChan).WithTimeout(time.Second * 2).ShouldNot(Receive())
		})
	})

	When("a file is chmoded", func() {
		var fileName string

		BeforeEach(func() {
			fileName = writeTempFile(configPath)
		})

		It("does not signal the event channel", func() {
			Expect(os.Chmod(fileName, 0o644)).To(Succeed())

			Consistently(eventChan).WithTimeout(time.Second * 2).ShouldNot(Receive())
		})
	})

	When("a file is renamed", func() {
		var fileName string

		BeforeEach(func() {
			fileName = writeTempFile(configPath)
		})

		It("only signals the event channel once", func() {
			Expect(os.Rename(fileName, filepath.Join(configPath, "newfile"))).To(Succeed())

			Eventually(eventChan).WithTimeout(time.Second * 2).Should(Receive(Equal(configPath)))
			Consistently(eventChan).WithTimeout(time.Second * 2).ShouldNot(Receive())
		})
	})
})

func writeTempFile(configPath string) string {
	configFileHandle, err := os.CreateTemp(configPath, "")
	Expect(err).NotTo(HaveOccurred())
	Expect(configFileHandle.Close()).To(Succeed())

	return configFileHandle.Name()
}
