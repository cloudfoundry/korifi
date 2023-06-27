package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/gomega/gstruct"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Task", func() {
	var (
		requestMethod    string
		requestPath      string
		appRepo          *fake.CFAppRepository
		taskRepo         *fake.CFTaskRepository
		requestValidator *fake.RequestValidator
	)

	BeforeEach(func() {
		requestMethod = http.MethodGet
		requestPath = "/v3/tasks"

		appRepo = new(fake.CFAppRepository)

		appRepo.GetAppReturns(repositories.AppRecord{
			GUID:      "the-app-guid",
			SpaceGUID: "the-space-guid",
			IsStaged:  true,
		}, nil)

		taskRepo = new(fake.CFTaskRepository)
		taskRepo.GetTaskReturns(repositories.TaskRecord{
			GUID:      "the-task-guid",
			SpaceGUID: "the-space-guid",
		}, nil)

		requestValidator = new(fake.RequestValidator)

		apiHandler := handlers.NewTask(*serverURL, appRepo, taskRepo, requestValidator)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestPath, strings.NewReader("the-json-body"))
		Expect(err).NotTo(HaveOccurred())
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("POST /v3/apps/:app-guid/tasks", func() {
		BeforeEach(func() {
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidateJSONPayloadStub(&payloads.TaskCreate{
				Command: "echo hello",
				Metadata: payloads.Metadata{
					Labels:      map[string]string{"env": "production"},
					Annotations: map[string]string{"hello": "there"},
				},
			})

			requestMethod = http.MethodPost
			requestPath = "/v3/apps/the-app-guid/tasks"

			taskRepo.CreateTaskReturns(repositories.TaskRecord{
				GUID: "the-task-guid",
			}, nil)
		})

		It("creates a task", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

			Expect(taskRepo.CreateTaskCallCount()).To(Equal(1))

			_, actualAuthInfo, createTaskMessage := taskRepo.CreateTaskArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(createTaskMessage.Command).To(Equal("echo hello"))
			Expect(createTaskMessage.AppGUID).To(Equal("the-app-guid"))
			Expect(createTaskMessage.SpaceGUID).To(Equal("the-space-guid"))
			Expect(createTaskMessage.Labels).To(Equal(map[string]string{"env": "production"}))
			Expect(createTaskMessage.Annotations).To(Equal(map[string]string{"hello": "there"}))

			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "the-task-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/tasks/the-task-guid"),
			)))
		})

		When("the app does not exist", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewNotFoundError(nil, repositories.AppResourceType))
			})

			It("returns a Not Found Error", func() {
				expectNotFoundError("App")
			})
		})

		When("the user cannot see the app", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns a Not Found error", func() {
				expectNotFoundError("App")
			})
		})

		When("getting the app fails", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
			})

			It("returns an Internal Server Error", func() {
				expectUnknownError()
			})
		})

		When("the app is not staged", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{
					GUID:      "the-app-guid",
					SpaceGUID: "the-space-guid",
					IsStaged:  false,
				}, nil)
			})

			It("returns an Unprocessable Entity error", func() {
				expectUnprocessableEntityError("Task must have a droplet. Assign current droplet to app.")
			})
		})

		When("the user cannot create tasks", func() {
			BeforeEach(func() {
				taskRepo.CreateTaskReturns(repositories.TaskRecord{}, apierrors.NewForbiddenError(nil, repositories.TaskResourceType))
			})

			It("returns a Forbidden error", func() {
				expectNotAuthorizedError()
			})
		})

		When("creating the task fails", func() {
			BeforeEach(func() {
				taskRepo.CreateTaskReturns(repositories.TaskRecord{}, errors.New("boom"))
			})

			It("returns an internal server error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("listing tasks", func() {
		BeforeEach(func() {
			taskRepo.ListTasksReturns([]repositories.TaskRecord{
				{GUID: "guid-1"},
				{GUID: "guid-2"},
			}, nil)
		})

		Describe("GET /v3/tasks", func() {
			It("lists the tasks", func() {
				Expect(taskRepo.ListTasksCallCount()).To(Equal(1))
				_, info, listMsg := taskRepo.ListTasksArgsForCall(0)
				Expect(info).To(Equal(authInfo))
				Expect(listMsg).To(Equal(repositories.ListTaskMessage{}))

				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/tasks"),
					MatchJSONPath("$.resources", HaveLen(2)),
					MatchJSONPath("$.resources[0].guid", "guid-1"),
					MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/tasks/guid-1"),
					MatchJSONPath("$.resources[1].guid", "guid-2"),
				)))
			})

			When("listing tasks fails", func() {
				BeforeEach(func() {
					taskRepo.ListTasksReturns(nil, errors.New("list-err"))
				})

				It("returns an Internal Server Error", func() {
					expectUnknownError()
				})
			})
		})

		Describe("GET /v3/apps/{app-guid}/tasks", func() {
			BeforeEach(func() {
				requestPath = "/v3/apps/the-app-guid/tasks"
			})

			It("lists the tasks", func() {
				Expect(taskRepo.ListTasksCallCount()).To(Equal(1))
				_, info, listMsg := taskRepo.ListTasksArgsForCall(0)
				Expect(info).To(Equal(authInfo))
				Expect(listMsg.AppGUIDs).To(ConsistOf("the-app-guid"))
				Expect(listMsg.SequenceIDs).To(BeEmpty())

				Expect(rr).To(HaveHTTPStatus(http.StatusOK))
				Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/apps/the-app-guid/tasks"),
					MatchJSONPath("$.resources", HaveLen(2)),
					MatchJSONPath("$.resources[0].guid", "guid-1"),
					MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/tasks/guid-1"),
					MatchJSONPath("$.resources[1].guid", "guid-2"),
				)))
			})

			When("the app does not exist", func() {
				BeforeEach(func() {
					appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewNotFoundError(nil, repositories.AppResourceType))
				})

				It("returns a Not Found Error", func() {
					expectNotFoundError("App")
				})
			})

			When("the user cannot see the app", func() {
				BeforeEach(func() {
					appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
				})

				It("returns a Not Found error", func() {
					expectNotFoundError("App")
				})
			})

			When("getting the app fails", func() {
				BeforeEach(func() {
					appRepo.GetAppReturns(repositories.AppRecord{}, errors.New("boom"))
				})

				It("returns an Internal Server Error", func() {
					expectUnknownError()
				})
			})

			When("filtering tasks by sequence ID", func() {
				BeforeEach(func() {
					requestPath = "/v3/apps/the-app-guid/tasks?sequence_ids=1,2"
				})

				It("provides a list task message with the sequence ids to the repository", func() {
					Expect(taskRepo.ListTasksCallCount()).To(Equal(1))
					_, _, listMsg := taskRepo.ListTasksArgsForCall(0)
					Expect(listMsg.SequenceIDs).To(ConsistOf(int64(1), int64(2)))
				})

				When("the sequence_ids parameter cannot be parsed to ints", func() {
					BeforeEach(func() {
						requestPath = "/v3/apps/the-app-guid/tasks?sequence_ids=asdf"
					})

					It("returns a bad request error", func() {
						expectBadRequestError()
					})
				})
			})

			When("the query string contains unsupported keys", func() {
				BeforeEach(func() {
					requestPath = "/v3/apps/the-app-guid/tasks?whatever=1"
				})

				It("returns an unknown key error", func() {
					expectUnknownKeyError("The query parameter is invalid: Valid parameters are: .*")
				})
			})

			When("listing tasks fails", func() {
				BeforeEach(func() {
					taskRepo.ListTasksReturns(nil, errors.New("list-err"))
				})

				It("returns an Internal Server Error", func() {
					expectUnknownError()
				})
			})
		})
	})

	Describe("GET /v3/tasks/:task-guid", func() {
		BeforeEach(func() {
			requestPath += "/the-task-guid"
		})

		It("gets the correct task", func() {
			Expect(taskRepo.GetTaskCallCount()).To(Equal(1))
			_, actualAuthInfo, taskGUID := taskRepo.GetTaskArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(taskGUID).To(Equal("the-task-guid"))
		})

		It("returns the task", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "the-task-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/tasks/the-task-guid"),
			)))
		})

		When("the FailureReason is set", func() {
			BeforeEach(func() {
				taskRepo.GetTaskReturns(repositories.TaskRecord{
					FailureReason: "bar",
				}, nil)
			})

			It("propagates to the response body", func() {
				Expect(rr).To(HaveHTTPBody(MatchJSONPath("$.result.failure_reason", "bar")))
			})
		})

		When("the task does not exist", func() {
			BeforeEach(func() {
				taskRepo.GetTaskReturns(repositories.TaskRecord{}, apierrors.NewNotFoundError(nil, "Task"))
			})

			It("returns a 404 Not Found", func() {
				expectNotFoundError("Task")
			})
		})

		When("the user is not authorized to get the task", func() {
			BeforeEach(func() {
				taskRepo.GetTaskReturns(repositories.TaskRecord{}, apierrors.NewForbiddenError(nil, "Task"))
			})

			It("returns a 404 Not Found", func() {
				expectNotFoundError("Task")
			})
		})
	})

	Describe("POST /v3/tasks/:taskGUID/actions/cancel", func() {
		BeforeEach(func() {
			taskRepo.CancelTaskReturns(repositories.TaskRecord{
				GUID: "the-task-guid",
			}, nil)

			requestMethod = http.MethodPost
			requestPath += "/the-task-guid/actions/cancel"
		})

		It("cancels the task", func() {
			Expect(taskRepo.GetTaskCallCount()).To(Equal(1))
			_, actualAuthInfo, taskGUID := taskRepo.GetTaskArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(taskGUID).To(Equal("the-task-guid"))

			Expect(taskRepo.CancelTaskCallCount()).To(Equal(1))
			_, actualAuthInfo, taskGUID = taskRepo.CancelTaskArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(taskGUID).To(Equal("the-task-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "the-task-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/tasks/the-task-guid"),
			)))
		})

		When("getting the task fails with forbidden error", func() {
			BeforeEach(func() {
				taskRepo.GetTaskReturns(repositories.TaskRecord{}, apierrors.NewForbiddenError(nil, repositories.TaskResourceType))
			})

			It("returns a Not Found Error", func() {
				expectNotFoundError("Task")
			})
		})

		When("getting the task fails with other error", func() {
			BeforeEach(func() {
				taskRepo.GetTaskReturns(repositories.TaskRecord{}, errors.New("task-get"))
			})

			It("returns an Internal Server Error", func() {
				expectUnknownError()
			})
		})

		When("cancelling the task fails", func() {
			BeforeEach(func() {
				taskRepo.CancelTaskReturns(repositories.TaskRecord{}, errors.New("task-cancel"))
			})

			It("returns an Internal Server Error", func() {
				expectUnknownError()
			})
		})

		When("cancelling the task is forbidden", func() {
			BeforeEach(func() {
				taskRepo.CancelTaskReturns(repositories.TaskRecord{}, apierrors.NewForbiddenError(nil, repositories.TaskResourceType))
			})

			It("returns an forbidden error", func() {
				expectNotAuthorizedError()
			})
		})
	})

	Describe("PUT /v3/tasks/:taskGUID/cancel", func() {
		BeforeEach(func() {
			taskRepo.CancelTaskReturns(repositories.TaskRecord{
				GUID: "the-task-guid",
			}, nil)

			requestMethod = http.MethodPut
			requestPath += "/the-task-guid/cancel"
		})

		It("cancels the task", func() {
			Expect(taskRepo.GetTaskCallCount()).To(Equal(1))
			_, actualAuthInfo, taskGUID := taskRepo.GetTaskArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(taskGUID).To(Equal("the-task-guid"))

			Expect(taskRepo.CancelTaskCallCount()).To(Equal(1))
			_, actualAuthInfo, taskGUID = taskRepo.CancelTaskArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(taskGUID).To(Equal("the-task-guid"))

			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "the-task-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/tasks/the-task-guid"),
			)))
		})
	})

	Describe("PATCH /v3/tasks/:guid", func() {
		BeforeEach(func() {
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidateJSONPayloadStub(&payloads.TaskUpdate{
				Metadata: payloads.MetadataPatch{
					Labels: map[string]*string{
						"env":                           tools.PtrTo("production"),
						"foo.example.com/my-identifier": tools.PtrTo("aruba"),
					},
					Annotations: map[string]*string{
						"hello":                       tools.PtrTo("there"),
						"foo.example.com/lorem-ipsum": tools.PtrTo("Lorem ipsum."),
					},
				},
			})

			taskRepo.PatchTaskMetadataReturns(repositories.TaskRecord{GUID: "the-task-guid"}, nil)

			requestMethod = http.MethodPatch
			requestPath += "/the-task-guid"
		})

		It("returns status 200 OK", func() {
		})

		It("patches the task with the new labels and annotations", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

			Expect(taskRepo.PatchTaskMetadataCallCount()).To(Equal(1))
			_, _, msg := taskRepo.PatchTaskMetadataArgsForCall(0)
			Expect(msg.TaskGUID).To(Equal("the-task-guid"))
			Expect(msg.SpaceGUID).To(Equal("the-space-guid"))
			Expect(msg.Annotations).To(HaveKeyWithValue("hello", PointTo(Equal("there"))))
			Expect(msg.Annotations).To(HaveKeyWithValue("foo.example.com/lorem-ipsum", PointTo(Equal("Lorem ipsum."))))
			Expect(msg.Labels).To(HaveKeyWithValue("env", PointTo(Equal("production"))))
			Expect(msg.Labels).To(HaveKeyWithValue("foo.example.com/my-identifier", PointTo(Equal("aruba"))))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "the-task-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/tasks/the-task-guid"),
			)))
		})

		When("the user doesn't have permission to get the task", func() {
			BeforeEach(func() {
				taskRepo.GetTaskReturns(repositories.TaskRecord{}, apierrors.NewForbiddenError(nil, repositories.TaskResourceType))
			})

			It("returns a not found error and doesn't call patch", func() {
				expectNotFoundError("Task")
				Expect(taskRepo.PatchTaskMetadataCallCount()).To(Equal(0))
			})
		})

		When("fetching the task errors", func() {
			BeforeEach(func() {
				taskRepo.GetTaskReturns(repositories.TaskRecord{}, errors.New("boom"))
			})

			It("returns an error and doesn't call patch", func() {
				expectUnknownError()
				Expect(taskRepo.PatchTaskMetadataCallCount()).To(Equal(0))
			})
		})

		When("patching the task errors", func() {
			BeforeEach(func() {
				taskRepo.PatchTaskMetadataReturns(repositories.TaskRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})
})
