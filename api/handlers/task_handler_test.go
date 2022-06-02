package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TaskHandler", func() {
	var (
		req      *http.Request
		appRepo  *fake.CFAppRepository
		taskRepo *fake.CFTaskRepository
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		taskRepo = new(fake.CFTaskRepository)
		decoderValidator, err := handlers.NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		appRepo.GetAppReturns(repositories.AppRecord{
			GUID:      "the-app-guid",
			SpaceGUID: "the-space-guid",
		}, nil)

		taskHandler := handlers.NewTaskHandler(*serverURL, appRepo, taskRepo, decoderValidator)
		taskHandler.RegisterRoutes(router)
	})

	JustBeforeEach(func() {
		router.ServeHTTP(rr, req)
	})

	Describe("POST /v3/apps/:app-guid/tasks", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/apps/the-app-guid/tasks", strings.NewReader(`{"command": "echo hello"}`))
			Expect(err).NotTo(HaveOccurred())

			taskRepo.CreateTaskReturns(repositories.TaskRecord{
				Name:    "the-task-name",
				GUID:    "the-task-guid",
				Command: "echo hello",
				AppGUID: "the-app-guid",
			}, nil)
		})

		It("returns a 201 Created response", func() {
			Expect(rr.Code).To(Equal(http.StatusCreated))
		})

		It("creates a task", func() {
			Expect(taskRepo.CreateTaskCallCount()).To(Equal(1))

			_, actualAuthInfo, createTaskMessage := taskRepo.CreateTaskArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(createTaskMessage.Command).To(Equal("echo hello"))
			Expect(createTaskMessage.AppGUID).To(Equal("the-app-guid"))
			Expect(createTaskMessage.SpaceGUID).To(Equal("the-space-guid"))
		})

		It("returns the created task", func() {
			Expect(rr.Body).To(MatchJSON(`{
              "name": "the-task-name",
              "guid": "the-task-guid",
              "command": "echo hello",
              "state": "RUNNING",
              "result": {
                "failure_reason": null
              },
              "relationships": {
                "app": {
                  "data": {
                    "guid": "the-app-guid"
                  }
                }
              },
              "links": {
                "self": {
                  "href": "https://api.example.org/v3/tasks/the-task-guid"
                },
                "app": {
                  "href": "https://api.example.org/v3/apps/the-app-guid"
                }
              }
            }`))
		})

		When("the app does not exist", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewNotFoundError(nil, repositories.TaskResourceType))
			})

			It("returns an Unprocessible Entity Error", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity))
			})
		})

		When("the user cannot see the app", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.TaskResourceType))
			})

			It("returns an Unprocessible Entity Error", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity))
			})
		})

		When("getting the app fails", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an internal server error", func() {
				Expect(rr.Code).To(Equal(http.StatusInternalServerError))
			})
		})

		When("the task creation fails with an api error", func() {
			BeforeEach(func() {
				taskRepo.CreateTaskReturns(repositories.TaskRecord{}, apierrors.NewForbiddenError(nil, repositories.TaskResourceType))
			})

			It("translates the error correctly", func() {
				Expect(rr.Code).To(Equal(http.StatusForbidden))
			})
		})

		When("creating the task fails", func() {
			BeforeEach(func() {
				taskRepo.CreateTaskReturns(repositories.TaskRecord{}, errors.New("boom"))
			})

			It("returns an internal server error", func() {
				Expect(rr.Code).To(Equal(http.StatusInternalServerError))
			})
		})
	})
})
