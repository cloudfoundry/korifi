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

var _ = Describe("FetchProcessStatsAction", func() {
	const (
		processGUID = "test-process-guid"
	)

	var (
		processRepo *fake.CFProcessRepository
		podRepo     *fake.PodRepository
		appRepo     *fake.CFAppRepository
		authInfo    authorization.Info

		fetchProcessStatsAction *FetchProcessStats

		appGUID         string
		spaceGUID       string
		responseRecords []repositories.PodStatsRecord
		responseErr     error
	)

	BeforeEach(func() {
		processRepo = new(fake.CFProcessRepository)
		podRepo = new(fake.PodRepository)
		appRepo = new(fake.CFAppRepository)
		authInfo = authorization.Info{Token: "a-token"}

		appGUID = "some-app-guid"
		spaceGUID = "some-space-guid"

		processRepo.FetchProcessReturns(repositories.ProcessRecord{
			AppGUID:   appGUID,
			SpaceGUID: spaceGUID,
		}, nil)

		podRepo.FetchPodStatsByAppGUIDReturns([]repositories.PodStatsRecord{
			{
				Type:  "web",
				Index: 0,
				State: "RUNNING",
			},
		}, nil)

		appRepo.FetchAppReturns(repositories.AppRecord{State: "STARTED"}, nil)

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
			Expect(appRepo.FetchAppCallCount()).To(Equal(1))
			_, actualAuthInfo, _ := appRepo.FetchAppArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
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
				appRepo.FetchAppReturns(repositories.AppRecord{State: "STOPPED"}, nil)
				responseRecords, responseErr = fetchProcessStatsAction.Invoke(context.Background(), authInfo, processGUID)
			})

			It("returns empty record", func() {
				Expect(responseRecords).To(BeEmpty())
			})
		})
	})

	When("on the unhappy path", func() {
		When("FetchProcess responds with some error", func() {
			BeforeEach(func() {
				processRepo.FetchProcessReturns(repositories.ProcessRecord{}, errors.New("some-error"))
				responseRecords, responseErr = fetchProcessStatsAction.Invoke(context.Background(), authInfo, processGUID)
			})

			It("returns a nil and error", func() {
				Expect(responseRecords).To(BeNil())
				Expect(responseErr).To(MatchError("some-error"))
			})
		})

		When("FetchApp responds with some error", func() {
			BeforeEach(func() {
				appRepo.FetchAppReturns(repositories.AppRecord{}, errors.New("some-error"))
				responseRecords, responseErr = fetchProcessStatsAction.Invoke(context.Background(), authInfo, processGUID)
			})

			It("returns a nil and error", func() {
				Expect(responseRecords).To(BeNil())
				Expect(responseErr).To(MatchError("some-error"))
			})
		})
	})
})
