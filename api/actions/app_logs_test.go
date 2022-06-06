package actions_test

import (
	"context"
	"errors"
	"time"

	. "code.cloudfoundry.org/korifi/api/actions"
	"code.cloudfoundry.org/korifi/api/actions/fake"
	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("ReadAppLogs", func() {
	const (
		appGUID   = "test-app-guid"
		buildGUID = "test-build-guid"

		spaceGUID = "test-space-guid"
	)

	var (
		appRepo   *fake.CFAppRepository
		buildRepo *fake.CFBuildRepository
		podRepo   *fake.PodRepository

		appLogs *AppLogs

		buildLogs, logs []repositories.LogRecord

		authInfo       authorization.Info
		requestPayload payloads.LogRead

		returnedRecords []repositories.LogRecord
		returnedErr     error
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		buildRepo = new(fake.CFBuildRepository)
		podRepo = new(fake.PodRepository)

		appLogs = NewAppLogs(appRepo, buildRepo, podRepo)

		appRepo.GetAppReturns(repositories.AppRecord{
			GUID:      appGUID,
			Revision:  "1",
			SpaceGUID: spaceGUID,
		}, nil)

		buildRepo.GetLatestBuildByAppGUIDReturns(repositories.BuildRecord{
			GUID:    buildGUID,
			AppGUID: appGUID,
		}, nil)

		nowTime := time.Now()
		buildLogs = []repositories.LogRecord{
			{
				Message:   "BuildMessage1",
				Timestamp: nowTime.UnixNano(),
			},
			{
				Message:   "BuildMessage2",
				Timestamp: nowTime.Add(time.Nanosecond).UnixNano(),
			},
		}
		buildRepo.GetBuildLogsReturns(buildLogs, nil)

		logs = []repositories.LogRecord{
			{
				Message:   "AppMessage1",
				Timestamp: nowTime.Add(time.Nanosecond * 2).UnixNano(),
			},
			{
				Message:   "AppMessage2",
				Timestamp: nowTime.Add(time.Nanosecond * 3).UnixNano(),
			},
		}
		podRepo.GetRuntimeLogsForAppReturns(logs, nil)

		requestPayload = payloads.LogRead{
			StartTime:     nil,
			EndTime:       nil,
			EnvelopeTypes: nil,
			Limit:         nil,
			Descending:    nil,
		}
		authInfo = authorization.Info{Token: "a-token"}
	})

	JustBeforeEach(func() {
		returnedRecords, returnedErr = appLogs.Read(context.Background(), logf.Log.WithName("testlogger"), authInfo, appGUID, requestPayload)
	})

	It("sets the log limit to 100 when not specified", func() {
		Expect(podRepo.GetRuntimeLogsForAppCallCount()).To(BeNumerically(">=", 1))
		_, _, _, message := podRepo.GetRuntimeLogsForAppArgsForCall(0)
		Expect(message.Limit).To(Equal(int64(100)))
	})

	It("returns the list of build and app records", func() {
		Expect(returnedErr).NotTo(HaveOccurred())
		Expect(returnedRecords).To(Equal(append(buildLogs, logs...)))
	})

	When("the limit is lower than the total number of logs available", func() {
		BeforeEach(func() {
			limit := int64(2)
			requestPayload.Limit = &limit
		})

		It("gives us the most recent logs up to the limit", func() {
			Expect(returnedErr).NotTo(HaveOccurred())
			Expect(returnedRecords).To(HaveLen(2))
			Expect(returnedRecords[0].Message).To(Equal("AppMessage1"))
			Expect(returnedRecords[1].Message).To(Equal("AppMessage2"))
		})

		When("the start time is set to the beginning of unix time according to the CF CLI", func() {
			BeforeEach(func() {
				theBigBang := int64(-6795364578871345152) // this is some date in 1754, which is what the CLI defaults to
				requestPayload.StartTime = &theBigBang
			})
			It("gives us the latest logs up to the log limit", func() {
				Expect(returnedErr).NotTo(HaveOccurred())
				Expect(returnedRecords).To(HaveLen(2))
				Expect(returnedRecords[0].Message).To(Equal("AppMessage1"))
				Expect(returnedRecords[1].Message).To(Equal("AppMessage2"))
			})
		})

		When("the build and run logs are chronologically interleaved", func() {
			BeforeEach(func() {
				buildLogs[1].Timestamp, logs[0].Timestamp = logs[0].Timestamp, buildLogs[1].Timestamp
			})
			It("gives us the most recent logs up to the limit", func() {
				Expect(returnedErr).NotTo(HaveOccurred())
				Expect(returnedRecords).To(HaveLen(2))
				Expect(returnedRecords[0].Message).To(Equal("BuildMessage2"))
				Expect(returnedRecords[1].Message).To(Equal("AppMessage2"))
			})
		})
	})

	When("the descending flag in the request is set to true", func() {
		BeforeEach(func() {
			descending := true
			requestPayload.Descending = &descending
		})

		It("returns the logs in the reverse order", func() {
			Expect(returnedErr).NotTo(HaveOccurred())
			Expect(returnedRecords).To(HaveLen(4))
			Expect(returnedRecords[0].Message).To(Equal("AppMessage2"))
			Expect(returnedRecords[3].Message).To(Equal("BuildMessage1"))
		})
	})

	When("the start time is newer than any of the log entries", func() {
		BeforeEach(func() {
			startTime := time.Now().Add(time.Minute).UnixNano()
			requestPayload.StartTime = &startTime
		})

		It("returns an empty list", func() {
			Expect(returnedErr).NotTo(HaveOccurred())
			Expect(returnedRecords).To(HaveLen(0))
		})
	})

	When("the start time is the same as the latest log entry", func() {
		BeforeEach(func() {
			startTime := logs[1].Timestamp
			requestPayload.StartTime = &startTime
		})

		It("returns only the latest entry", func() {
			Expect(returnedErr).NotTo(HaveOccurred())
			Expect(returnedRecords).To(HaveLen(1))
		})
	})

	When("GetApp returns a Forbidden error", func() {
		BeforeEach(func() {
			appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(errors.New("blah"), repositories.AppResourceType))
		})
		It("returns a NotFound error", func() {
			Expect(returnedErr).To(HaveOccurred())
			Expect(returnedErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
		})
	})

	When("GetApp returns a random error", func() {
		var getAppError error
		BeforeEach(func() {
			getAppError = errors.New("blah")
			appRepo.GetAppReturns(repositories.AppRecord{}, getAppError)
		})
		It("returns the error transparently", func() {
			Expect(returnedErr).To(HaveOccurred())
			Expect(returnedErr).To(Equal(getAppError))
		})
	})

	When("GetLatestBuildByAppGUIDReturns returns a Forbidden error", func() {
		BeforeEach(func() {
			buildRepo.GetLatestBuildByAppGUIDReturns(repositories.BuildRecord{}, apierrors.NewForbiddenError(errors.New("blah"), repositories.BuildResourceType))
		})
		It("returns a NotFound error", func() {
			Expect(returnedErr).To(HaveOccurred())
			Expect(returnedErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
		})
	})

	When("GetLatestBuildByAppGUIDReturns returns a random error", func() {
		var getLatestBuildByAppGUID error
		BeforeEach(func() {
			getLatestBuildByAppGUID = errors.New("blah")
			buildRepo.GetLatestBuildByAppGUIDReturns(repositories.BuildRecord{}, getLatestBuildByAppGUID)
		})
		It("returns the error transparently", func() {
			Expect(returnedErr).To(HaveOccurred())
			Expect(returnedErr).To(Equal(getLatestBuildByAppGUID))
		})
	})

	When("GetBuildLogsReturns returns an error", func() {
		var getBuildLogsReturns error
		BeforeEach(func() {
			getBuildLogsReturns = errors.New("blah")
			buildRepo.GetBuildLogsReturns(nil, getBuildLogsReturns)
		})
		It("returns the error transparently", func() {
			Expect(returnedErr).To(HaveOccurred())
			Expect(returnedErr).To(Equal(getBuildLogsReturns))
		})
	})

	When("GetRuntimeLogsForAppReturns returns an error", func() {
		var getRuntimeLogsReturns error
		BeforeEach(func() {
			getRuntimeLogsReturns = errors.New("blah")
			podRepo.GetRuntimeLogsForAppReturns(nil, getRuntimeLogsReturns)
		})
		It("returns the error transparently", func() {
			Expect(returnedErr).To(HaveOccurred())
			Expect(returnedErr).To(Equal(getRuntimeLogsReturns))
		})
	})
})
