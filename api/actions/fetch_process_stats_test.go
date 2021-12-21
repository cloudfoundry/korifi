package actions_test

import (
	"context"
	"errors"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	"code.cloudfoundry.org/cf-k8s-controllers/api/actions/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetProcessStatsAction", func() {
	const (
		processGUID = "test-process-guid"
	)

	var (
		processRepo *fake.CFProcessRepository
		podRepo     *fake.PodRepository
		appRepo     *fake.CFAppRepository
		authInfo    authorization.Info

		fetchProcessStatsAction *FetchProcessStats

		appGUID          string
		appRevision      string
		spaceGUID        string
		desiredInstances int
		processType      string
		responseRecords  []repositories.PodStatsRecord
		responseErr      error
	)

	BeforeEach(func() {
		processRepo = new(fake.CFProcessRepository)
		podRepo = new(fake.PodRepository)
		appRepo = new(fake.CFAppRepository)
		authInfo = authorization.Info{Token: "a-token"}

		appGUID = "some-app-guid"
		appRevision = "1"
		spaceGUID = "some-space-guid"
		desiredInstances = 1
		processType = "web"

		processRepo.GetProcessReturns(repositories.ProcessRecord{
			AppGUID:          appGUID,
			SpaceGUID:        spaceGUID,
			DesiredInstances: desiredInstances,
			Type:             processType,
		}, nil)

		podRepo.ListPodStatsReturns([]repositories.PodStatsRecord{
			{
				Type:  processType,
				Index: 0,
				State: "RUNNING",
			},
		}, nil)

		appRepo.GetAppReturns(repositories.AppRecord{
			State:    "STARTED",
			Revision: appRevision,
		}, nil)

		fetchProcessStatsAction = NewFetchProcessStats(processRepo, podRepo, appRepo)
	})

	When("on the happy path", func() {
		BeforeEach(func() {
			responseRecords, responseErr = fetchProcessStatsAction.Invoke(context.Background(), authInfo, processGUID)
		})

		It("does not return an error", func() {
			Expect(responseErr).ToNot(HaveOccurred())
		})

		It("passes the authInfo to the repo call", func() {
			Expect(appRepo.GetAppCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := appRepo.GetAppArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
		})

		It("calls the fetch pod stats with expected message data", func() {
			_, _, message := podRepo.ListPodStatsArgsForCall(0)
			Expect(message).To(Equal(repositories.ListPodStatsMessage{
				Namespace:   spaceGUID,
				AppGUID:     appGUID,
				Instances:   desiredInstances,
				ProcessType: processType,
				AppRevision: appRevision,
			}))
		})

		It("fetches the stats for a process associated with the GUID", func() {
			Expect(responseRecords).To(HaveLen(1))
			Expect(responseRecords).To(ConsistOf(repositories.PodStatsRecord{
				Type:  "web",
				Index: 0,
				State: "RUNNING",
			}))
		})

		When("the app is STOPPED", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{State: "STOPPED"}, nil)
				responseRecords, responseErr = fetchProcessStatsAction.Invoke(context.Background(), authInfo, processGUID)
			})

			It("returns empty record", func() {
				Expect(responseRecords).To(BeEmpty())
			})
		})
	})

	When("on the unhappy path", func() {
		When("GetProcess responds with some error", func() {
			BeforeEach(func() {
				processRepo.GetProcessReturns(repositories.ProcessRecord{}, errors.New("some-error"))
				responseRecords, responseErr = fetchProcessStatsAction.Invoke(context.Background(), authInfo, processGUID)
			})

			It("returns a nil and error", func() {
				Expect(responseRecords).To(BeNil())
				Expect(responseErr).To(MatchError("some-error"))
			})
		})

		When("GetApp responds with some error", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("some-error"))
				responseRecords, responseErr = fetchProcessStatsAction.Invoke(context.Background(), authInfo, processGUID)
			})

			It("returns a nil and error", func() {
				Expect(responseRecords).To(BeNil())
				Expect(responseErr).To(MatchError("some-error"))
			})
		})
	})
})
