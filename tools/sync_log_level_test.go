package tools_test

import (
	"context"
	"errors"

	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/fake"
	"go.uber.org/zap"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SyncLogLevel", func() {
	var (
		atomicLevel    zap.AtomicLevel
		cancelFunc     context.CancelFunc
		eventChan      chan string
		logLevelGetter *fake.LogLevelGetter
	)

	BeforeEach(func() {
		atomicLevel = zap.NewAtomicLevelAt(zap.InfoLevel)
		eventChan = make(chan string)
		logLevelGetter = new(fake.LogLevelGetter)

		var subctx context.Context
		subctx, cancelFunc = context.WithCancel(context.Background())

		go tools.SyncLogLevel(subctx, GinkgoLogr, eventChan, atomicLevel, logLevelGetter.Spy)
	})

	AfterEach(func() {
		cancelFunc()
	})

	When("an event is received on the channel", func() {
		BeforeEach(func() {
			logLevelGetter.Returns(zap.DebugLevel, nil)
			Eventually(eventChan).Should(BeSent("/path/to/config"))
		})

		It("updates the atomic log level", func() {
			Eventually(atomicLevel.Level).Should(Equal(zap.DebugLevel))

			Expect(logLevelGetter.CallCount()).To(Equal(1))
			path := logLevelGetter.ArgsForCall(0)
			Expect(path).To(Equal("/path/to/config"))
		})
	})

	When("the log level getter returns an error", func() {
		BeforeEach(func() {
			logLevelGetter.Returns(zap.InfoLevel, errors.New("failed"))
			Eventually(eventChan).Should(BeSent("/path/to/config"))
		})

		It("ignores the event from the channel", func() {
			Consistently(atomicLevel.Level).Should(Equal(zap.InfoLevel))
		})

		When("another event is received", func() {
			BeforeEach(func() {
				logLevelGetter.Returns(zap.WarnLevel, nil)
				Eventually(eventChan).Should(BeSent("/path/to/config"))
			})

			It("updates the atomic log level", func() {
				Eventually(atomicLevel.Level).Should(Equal(zap.WarnLevel))
			})
		})
	})
})
