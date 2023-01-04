package actions_test

import (
	"context"
	"errors"

	. "code.cloudfoundry.org/korifi/api/actions"
	"code.cloudfoundry.org/korifi/api/actions/shared/fake"
	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Process Scaler", func() {
	const (
		testAppGUID          = "test-app-guid"
		testProcessGUID      = "test-process-guid"
		testProcessSpaceGUID = "test-namespace"

		initialInstances   = 1
		initialMemoryMB    = 256
		initialDiskQuotaMB = 1024

		processType = "web"
	)
	var (
		appRepo     *fake.CFAppRepository
		processRepo *fake.CFProcessRepository
		authInfo    authorization.Info

		updatedProcessRecord repositories.ProcessRecord

		processScaler *ProcessScaler

		testScale repositories.ProcessScaleValues

		responseRecord repositories.ProcessRecord
		responseErr    error
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		processRepo = new(fake.CFProcessRepository)
		authInfo = authorization.Info{Token: "a-token"}

		newInstances := 10
		var newMemoryMB int64 = 256
		var newDiskMB int64 = 1024

		testScale = repositories.ProcessScaleValues{
			Instances: &newInstances,
			MemoryMB:  &newMemoryMB,
			DiskMB:    &newDiskMB,
		}

		updatedProcessRecord = repositories.ProcessRecord{
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
		processRepo.ScaleProcessReturns(updatedProcessRecord, nil)

		processScaler = NewProcessScaler(appRepo, processRepo)
	})

	Describe("ScaleAppProcess", func() {
		BeforeEach(func() {
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
			processRepo.ListProcessesReturns([]repositories.ProcessRecord{processRecord}, nil)

			appRepo.GetAppReturns(repositories.AppRecord{
				Name:      testAppGUID,
				GUID:      testAppGUID,
				SpaceGUID: testProcessSpaceGUID,
			}, nil)
		})

		JustBeforeEach(func() {
			responseRecord, responseErr = processScaler.ScaleAppProcess(context.Background(), authInfo, testAppGUID, processType, testScale)
		})

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
		It("calls scale process correctly", func() {
			Expect(processRepo.ScaleProcessCallCount()).ToNot(BeZero())
			_, _, message := processRepo.ScaleProcessArgsForCall(0)
			Expect(message.GUID).To(Equal(testProcessGUID))
			Expect(message.SpaceGUID).To(Equal(testProcessSpaceGUID))
			Expect(message.ProcessScaleValues).To(Equal(testScale))
		})

		It("transparently returns a record from repositories.ProcessScaleValues", func() {
			Expect(responseRecord).To(Equal(updatedProcessRecord))
		})

		When("there is an error fetching the app and", func() {
			When("the returned error from GetApp is forbidden", func() {
				BeforeEach(func() {
					appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
				})

				It("wraps the error as NotFound", func() {
					Expect(responseErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})

			When("the error is some error other than forbidden", func() {
				BeforeEach(func() {
					appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("some-other-error"))
				})

				It("passes through the error", func() {
					Expect(responseErr).To(MatchError("some-other-error"))
				})
			})
		})

		When("there is an error fetching the processes for an app and", func() {
			When("the returned error from ListProcesses is forbidden", func() {
				BeforeEach(func() {
					processRepo.ListProcessesReturns([]repositories.ProcessRecord{}, apierrors.NewForbiddenError(nil, repositories.ProcessResourceType))
				})

				It("wraps the error as NotFound", func() {
					Expect(responseErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})

			When("the error is some other error", func() {
				BeforeEach(func() {
					processRepo.ListProcessesReturns([]repositories.ProcessRecord{}, errors.New("some-other-error"))
				})

				It("passes through the error", func() {
					Expect(responseErr).To(MatchError("some-other-error"))
				})
			})
		})

		When("there is an error updating the process", func() {
			BeforeEach(func() {
				processRepo.ScaleProcessReturns(repositories.ProcessRecord{}, errors.New("some-error"))
			})

			It("returns the error", func() {
				Expect(responseErr).To(MatchError("some-error"))
			})
		})
	})

	Describe("ScaleProcessAction", func() {
		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{
				GUID:             testProcessGUID,
				SpaceGUID:        testProcessSpaceGUID,
				DesiredInstances: initialInstances,
				MemoryMB:         initialMemoryMB,
				DiskQuotaMB:      initialDiskQuotaMB,
			}, nil)
		})

		JustBeforeEach(func() {
			responseRecord, responseErr = processScaler.ScaleProcess(context.Background(), authInfo, testProcessGUID, testScale)
		})

		It("does not return an error", func() {
			Expect(responseErr).ToNot(HaveOccurred())
		})
		It("fetches the process associated with the GUID", func() {
			Expect(processRepo.GetProcessCallCount()).ToNot(BeZero())
			_, _, processGUID := processRepo.GetProcessArgsForCall(0)
			Expect(processGUID).To(Equal(testProcessGUID))
		})

		It("fabricates a ProcessScaleValues using the inputs and the process GUID and looked-up space", func() {
			Expect(processRepo.ScaleProcessCallCount()).ToNot(BeZero())
			_, actualAuthInfo, scaleProcessMessage := processRepo.ScaleProcessArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(scaleProcessMessage.GUID).To(Equal(testProcessGUID))
			Expect(scaleProcessMessage.SpaceGUID).To(Equal(testProcessSpaceGUID))
			Expect(scaleProcessMessage.Instances).To(Equal(testScale.Instances))
			Expect(scaleProcessMessage.DiskMB).To(Equal(testScale.DiskMB))
			Expect(scaleProcessMessage.MemoryMB).To(Equal(testScale.MemoryMB))
		})

		It("transparently returns a record from repositories.ProcessScaleValues", func() {
			Expect(responseRecord).To(Equal(updatedProcessRecord))
		})

		When("there is an error fetching the process and", func() {
			When(`the error is "Forbidden"`, func() {
				BeforeEach(func() {
					processRepo.GetProcessReturns(repositories.ProcessRecord{}, apierrors.NewForbiddenError(nil, repositories.ProcessResourceType))
				})

				It("wraps the error as NotFound", func() {
					Expect(responseErr).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
				})
			})

			When("the error is some other error", func() {
				BeforeEach(func() {
					processRepo.GetProcessReturns(repositories.ProcessRecord{}, errors.New("some-error"))
				})

				It("passes through the error", func() {
					Expect(responseErr).To(MatchError("some-error"))
				})
			})
		})

		When("there is an error updating the process", func() {
			BeforeEach(func() {
				processRepo.ScaleProcessReturns(repositories.ProcessRecord{}, errors.New("some-error"))
			})

			It("passes through the error", func() {
				Expect(responseErr).To(MatchError("some-error"))
			})
		})
	})
})
