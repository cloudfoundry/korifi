package util_test

import (
	"errors"

	"code.cloudfoundry.org/korifi/statefulset-runner/util"
	"code.cloudfoundry.org/korifi/statefulset-runner/util/utilfakes"
	"code.cloudfoundry.org/lager"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//counterfeiter:generate -o utilfakes/fake_logger.go code.cloudfoundry.org/lager.Logger

var _ = Describe("LagerLogr", func() {
	var (
		fakeLogger *utilfakes.FakeLogger
		lagerLogr  logr.Logger
	)

	BeforeEach(func() {
		fakeLogger = new(utilfakes.FakeLogger)
		lagerLogr = util.NewLagerLogr(fakeLogger)
	})

	It("is always enabled", func() {
		Expect(lagerLogr.Enabled()).To(BeTrue())
	})

	Describe("Info", func() {
		JustBeforeEach(func() {
			lagerLogr.Info("some-message", "some-key", "some-value")
		})

		It("delegates to lagger's Info", func() {
			Expect(fakeLogger.InfoCallCount()).To(Equal(1))
			actualMsg, actualLagerData := fakeLogger.InfoArgsForCall(0)
			Expect(actualMsg).To(Equal("some-message"))
			Expect(actualLagerData).To(HaveLen(1))
			Expect(actualLagerData[0]).To(Equal(lager.Data{"some-key": "some-value"}))
		})
	})

	Describe("Error", func() {
		JustBeforeEach(func() {
			lagerLogr.Error(errors.New("some-error"), "some-message", "some-key", "some-value")
		})

		It("delegates to lagger's Error", func() {
			Expect(fakeLogger.ErrorCallCount()).To(Equal(1))
			actualMsg, actualErr, actualLagerData := fakeLogger.ErrorArgsForCall(0)
			Expect(actualErr).To(MatchError("some-error"))
			Expect(actualMsg).To(Equal("some-message"))
			Expect(actualLagerData).To(HaveLen(1))
			Expect(actualLagerData[0]).To(Equal(lager.Data{"some-key": "some-value"}))
		})
	})

	Describe("WithValues", func() {
		It("adds data to the lager logger", func() {
			lagerLogr.WithValues("additional-key", "additional-value")

			Expect(fakeLogger.WithDataCallCount()).To(Equal(1))
			actualData := fakeLogger.WithDataArgsForCall(0)
			Expect(actualData).To(Equal(lager.Data{
				"additional-key": "additional-value",
			}))
		})
	})

	Describe("WithName", func() {
		It("creates a new lager session", func() {
			lagerLogr.WithName("foo")

			Expect(fakeLogger.SessionCallCount()).To(Equal(1))
			actualName, actualData := fakeLogger.SessionArgsForCall(0)
			Expect(actualName).To(Equal("foo"))
			Expect(actualData).To(BeEmpty())
		})
	})
})
