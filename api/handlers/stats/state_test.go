package stats_test

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/api/handlers/stats"
	"code.cloudfoundry.org/korifi/api/handlers/stats/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

var _ = Describe("State", func() {
	var (
		processRepo    *fake.CFProcessRepository
		stateCollector *stats.ProcessInstancesStateCollector
		instancesState []stats.ProcessInstanceState
		stateErr       error
	)

	BeforeEach(func() {
		processRepo = new(fake.CFProcessRepository)
		processRepo.GetProcessReturns(repositories.ProcessRecord{
			Type: "web",
			InstancesState: map[string]korifiv1alpha1.InstanceState{
				"1": korifiv1alpha1.InstanceStateCrashed,
				"2": korifiv1alpha1.InstanceStateRunning,
			},
		}, nil)

		stateCollector = stats.NewProcessInstanceStateCollector(processRepo)
	})

	JustBeforeEach(func() {
		instancesState, stateErr = stateCollector.CollectProcessInstancesStates(ctx, "process-guid")
	})

	It("gets the process from the repository", func() {
		Expect(stateErr).NotTo(HaveOccurred())

		Expect(processRepo.GetProcessCallCount()).To(Equal(1))
		_, actualAuthInfo, actualProcessGUID := processRepo.GetProcessArgsForCall(0)
		Expect(actualAuthInfo).To(Equal(authInfo))
		Expect(actualProcessGUID).To(Equal("process-guid"))
	})

	When("getting the process fails", func() {
		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{}, errors.New("get-process-err"))
		})

		It("returns an error", func() {
			Expect(stateErr).To(MatchError(ContainSubstring("get-process-err")))
		})
	})

	It("returns instances state", func() {
		Expect(stateErr).NotTo(HaveOccurred())
		Expect(instancesState).To(ConsistOf(
			stats.ProcessInstanceState{
				ID:    1,
				Type:  "web",
				State: korifiv1alpha1.InstanceStateCrashed,
			},
			stats.ProcessInstanceState{
				ID:    2,
				Type:  "web",
				State: korifiv1alpha1.InstanceStateRunning,
			},
		))
	})

	When("instance id is not a number", func() {
		BeforeEach(func() {
			processRepo.GetProcessReturns(repositories.ProcessRecord{
				Type: "web",
				InstancesState: map[string]korifiv1alpha1.InstanceState{
					"one": korifiv1alpha1.InstanceStateCrashed,
				},
			}, nil)
		})

		It("returns an error", func() {
			Expect(stateErr).To(MatchError(ContainSubstring("one")))
		})
	})
})
