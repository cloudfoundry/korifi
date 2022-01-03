package actions_test

import (
	"context"
	"errors"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	"code.cloudfoundry.org/cf-k8s-controllers/api/actions/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ScaleProcessAction", func() {
	const (
		testProcessGUID      = "test-process-guid"
		testProcessSpaceGUID = "test-namespace"

		initialInstances   = 1
		initialMemoryMB    = 256
		initialDiskQuotaMB = 1024
	)
	var (
		processRepo *fake.CFProcessRepository

		updatedProcessRecord *repositories.ProcessRecord

		scaleProcessAction *ScaleProcess

		authInfo  authorization.Info
		testScale *repositories.ProcessScaleValues

		responseRecord repositories.ProcessRecord
		responseErr    error
	)

	BeforeEach(func() {
		processRepo = new(fake.CFProcessRepository)
		authInfo = authorization.Info{Token: "a-token"}

		processRepo.GetProcessReturns(repositories.ProcessRecord{
			GUID:             testProcessGUID,
			SpaceGUID:        testProcessSpaceGUID,
			DesiredInstances: initialInstances,
			MemoryMB:         initialMemoryMB,
			DiskQuotaMB:      initialDiskQuotaMB,
		}, nil)

		updatedProcessRecord = &repositories.ProcessRecord{
			GUID:             testProcessGUID,
			SpaceGUID:        testProcessSpaceGUID,
			AppGUID:          "some-app-guid",
			Type:             "web",
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
		processRepo.ScaleProcessReturns(*updatedProcessRecord, nil)

		newInstances := 10
		var newMemoryMB int64 = 256
		var newDiskMB int64 = 1024

		testScale = &repositories.ProcessScaleValues{
			Instances: &newInstances,
			MemoryMB:  &newMemoryMB,
			DiskMB:    &newDiskMB,
		}

		scaleProcessAction = NewScaleProcess(processRepo)
	})

	JustBeforeEach(func() {
		responseRecord, responseErr = scaleProcessAction.Invoke(context.Background(), authInfo, testProcessGUID, *testScale)
	})

	When("on the happy path", func() {
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
			Expect(responseRecord).To(Equal(*updatedProcessRecord))
		})
	})

	When("there is an error fetching the process and", func() {
		When("the error is \"not found\"", func() {
			var toReturnErr error
			BeforeEach(func() {
				toReturnErr = repositories.NotFoundError{}
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, toReturnErr)
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
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, toReturnErr)
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
		var toReturnErr error
		BeforeEach(func() {
			toReturnErr = errors.New("some-other-error")
			processRepo.ScaleProcessReturns(repositories.ProcessRecord{}, toReturnErr)
		})
		It("returns an empty record", func() {
			Expect(responseRecord).To(Equal(repositories.ProcessRecord{}))
		})
		It("passes through the error", func() {
			Expect(responseErr).To(Equal(toReturnErr))
		})
	})
})
