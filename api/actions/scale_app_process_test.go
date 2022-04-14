package actions_test

import (
	"context"
	"errors"

	. "code.cloudfoundry.org/korifi/api/actions"
	"code.cloudfoundry.org/korifi/api/actions/fake"
	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ScaleAppProcessAction", func() {
	const (
		testAppGUID          = "test-app-guid"
		testProcessGUID      = "test-process-guid"
		testProcessSpaceGUID = "test-namespace"

		initialInstances   = 1
		initialMemoryMB    = 256
		initialDiskQuotaMB = 1024
	)
	var (
		appRepo     *fake.CFAppRepository
		processRepo *fake.CFProcessRepository
		processType string
		authInfo    authorization.Info

		updatedProcessRecord *repositories.ProcessRecord

		scaleProcessAction    *fake.ScaleProcess
		scaleAppProcessAction *ScaleAppProcess

		testScale *repositories.ProcessScaleValues

		responseRecord repositories.ProcessRecord
		responseErr    error
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		processRepo = new(fake.CFProcessRepository)
		processType = "web"
		authInfo = authorization.Info{Token: "a-token"}

		processRecord := repositories.ProcessRecord{
			GUID:             testProcessGUID,
			SpaceGUID:        testProcessSpaceGUID,
			AppGUID:          testAppGUID,
			Type:             processType,
			Command:          "some command",
			DesiredInstances: initialInstances,
			MemoryMB:         initialMemoryMB,
			DiskQuotaMB:      initialDiskQuotaMB,
		}

		appRepo.GetAppReturns(repositories.AppRecord{
			Name:      testAppGUID,
			GUID:      testAppGUID,
			SpaceGUID: testProcessSpaceGUID,
		}, nil)

		processRepo.ListProcessesReturns([]repositories.ProcessRecord{processRecord}, nil)

		newInstances := 10
		var newMemoryMB int64 = 256
		var newDiskMB int64 = 1024

		testScale = &repositories.ProcessScaleValues{
			Instances: &newInstances,
			MemoryMB:  &newMemoryMB,
			DiskMB:    &newDiskMB,
		}
		scaleProcessAction = &fake.ScaleProcess{}

		updatedProcessRecord = &repositories.ProcessRecord{
			GUID:             testProcessGUID,
			SpaceGUID:        testProcessSpaceGUID,
			AppGUID:          testAppGUID,
			Type:             processType,
			Command:          "some-command",
			DesiredInstances: initialInstances,
			MemoryMB:         initialMemoryMB,
			DiskQuotaMB:      initialDiskQuotaMB,
			Ports:            []int32{8080},
			HealthCheck: repositories.HealthCheck{
				Type: "port",
				Data: repositories.HealthCheckData{},
			},
			Labels:      nil,
			Annotations: nil,
			CreatedAt:   "1906-04-18T13:12:00Z",
			UpdatedAt:   "1906-04-18T13:12:01Z",
		}
		scaleProcessAction.Returns(*updatedProcessRecord, nil)

		scaleAppProcessAction = NewScaleAppProcess(appRepo, processRepo, scaleProcessAction.Spy)
	})

	JustBeforeEach(func() {
		responseRecord, responseErr = scaleAppProcessAction.Invoke(context.Background(), authInfo, testAppGUID, processType, *testScale)
	})

	When("on the happy path", func() {
		It("does not return an error", func() {
			Expect(responseErr).ToNot(HaveOccurred())
		})
		It("fetches the app associated with the GUID", func() {
			Expect(appRepo.GetAppCallCount()).ToNot(BeZero())
			_, actualAuthInfo, appGUID := appRepo.GetAppArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(appGUID).To(Equal(testAppGUID))
		})
		It("fetches the processes associated with the App GUID", func() {
			Expect(processRepo.ListProcessesCallCount()).ToNot(BeZero())
			_, _, message := processRepo.ListProcessesArgsForCall(0)
			Expect(message.AppGUIDs[0]).To(Equal(testAppGUID))
			Expect(message.SpaceGUID).To(Equal(testProcessSpaceGUID))
		})
		It("fabricates a ProcessScaleValues using the inputs and the process GUID and looked-up space", func() {
			Expect(scaleProcessAction.CallCount()).ToNot(BeZero())
			_, _, scaleProcessGUID, scaleProcessMessage := scaleProcessAction.ArgsForCall(0)
			Expect(scaleProcessGUID).To(Equal(testProcessGUID))
			Expect(scaleProcessMessage.Instances).To(Equal(testScale.Instances))
			Expect(scaleProcessMessage.DiskMB).To(Equal(testScale.DiskMB))
			Expect(scaleProcessMessage.MemoryMB).To(Equal(testScale.MemoryMB))
		})
		It("transparently returns a record from repositories.ProcessScaleValues", func() {
			Expect(responseRecord).To(Equal(*updatedProcessRecord))
		})
	})

	When("there is an error fetching the app and", func() {
		When("the error is \"not found\"", func() {
			var toReturnErr error
			BeforeEach(func() {
				toReturnErr = apierrors.NewNotFoundError(nil, repositories.AppResourceType)
				appRepo.GetAppReturns(repositories.AppRecord{}, toReturnErr)
			})
			It("returns an empty record", func() {
				Expect(responseRecord).To(Equal(repositories.ProcessRecord{}))
			})
			It("passes through the error", func() {
				Expect(responseErr).To(Equal(toReturnErr))
			})
		})

		When("the returned error from GetApp is forbidden", func() {
			BeforeEach(func() {
				toReturnErr := apierrors.NewForbiddenError(nil, repositories.AppResourceType)
				appRepo.GetAppReturns(repositories.AppRecord{}, toReturnErr)
			})
			It("wraps the error as NotFound", func() {
				Expect(responseErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})

		When("the returned error from ListProcesses is forbidden", func() {
			BeforeEach(func() {
				toReturnErr := apierrors.NewForbiddenError(nil, repositories.ProcessResourceType)
				processRepo.ListProcessesReturns([]repositories.ProcessRecord{}, toReturnErr)
			})
			It("wraps the error as NotFound", func() {
				Expect(responseErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
			})
		})

		When("the error is some error other than forbidden", func() {
			var toReturnErr error
			BeforeEach(func() {
				toReturnErr = errors.New("some-other-error")
				appRepo.GetAppReturns(repositories.AppRecord{}, toReturnErr)
			})
			It("returns an empty record", func() {
				Expect(responseRecord).To(Equal(repositories.ProcessRecord{}))
			})
			It("passes through the error", func() {
				Expect(responseErr).To(Equal(toReturnErr))
			})
		})
	})

	When("there is an error fetching the processes for an app and", func() {
		When("the error is some other error", func() {
			var toReturnErr error
			BeforeEach(func() {
				toReturnErr = errors.New("some-other-error")
				processRepo.ListProcessesReturns([]repositories.ProcessRecord{}, toReturnErr)
			})
			It("returns an empty record", func() {
				Expect(responseRecord).To(Equal(repositories.ProcessRecord{}))
			})
			It("passes through the error", func() {
				Expect(responseErr).To(Equal(toReturnErr))
			})
		})
	})

	When("there is an error updating the process", func() {
		When("the error is \"not found\"", func() {
			var toReturnErr error
			BeforeEach(func() {
				toReturnErr = apierrors.NewNotFoundError(nil, repositories.ProcessResourceType)
				scaleProcessAction.Returns(repositories.ProcessRecord{}, toReturnErr)
			})
			It("returns an empty record", func() {
				Expect(responseRecord).To(Equal(repositories.ProcessRecord{}))
			})
			It("passes through the error", func() {
				Expect(responseErr).To(Equal(toReturnErr))
			})
		})

		When("the error is some other error", func() {
			var toReturnErr error
			BeforeEach(func() {
				toReturnErr = errors.New("some-other-error")
				scaleProcessAction.Returns(repositories.ProcessRecord{}, toReturnErr)
			})
			It("returns an empty record", func() {
				Expect(responseRecord).To(Equal(repositories.ProcessRecord{}))
			})
			It("passes through the error", func() {
				Expect(responseErr).To(Equal(toReturnErr))
			})
		})
	})
})
