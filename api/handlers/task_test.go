package handlers_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Task", func() {
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
			IsStaged:  true,
		}, nil)

		apiHandler := handlers.NewTask(*serverURL, appRepo, taskRepo, decoderValidator)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("POST /v3/apps/:app-guid/tasks", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/apps/the-app-guid/tasks", strings.NewReader(`{"command": "echo hello"}`))
			Expect(err).NotTo(HaveOccurred())

			taskRepo.CreateTaskReturns(repositories.TaskRecord{
				Name:              "the-task-name",
				GUID:              "the-task-guid",
				Command:           "echo hello",
				AppGUID:           "the-app-guid",
				DropletGUID:       "the-droplet-guid",
				SequenceID:        123456,
				CreationTimestamp: time.Date(2022, 6, 14, 13, 22, 34, 0, time.UTC),
				MemoryMB:          256,
				DiskMB:            128,
				State:             "task-created",
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
              "sequence_id": 123456,
              "created_at": "2022-06-14T13:22:34Z",
              "updated_at": "2022-06-14T13:22:34Z",
              "memory_in_mb": 256,
              "disk_in_mb": 128,
              "droplet_guid": "the-droplet-guid",
              "state": "task-created",
              "relationships": {
                "app": {
                  "data": {
                    "guid": "the-app-guid"
                  }
                }
              },
              "result": {
                "failure_reason": null
              },
              "links": {
                "self": {
                  "href": "https://api.example.org/v3/tasks/the-task-guid"
                },
                "app": {
                  "href": "https://api.example.org/v3/apps/the-app-guid"
                },
                "droplet": {
                  "href": "https://api.example.org/v3/droplets/the-droplet-guid"
                },
                "cancel": {
                  "href": "https://api.example.org/v3/tasks/the-task-guid/actions/cancel",
                  "method": "POST"
                }
              }
            }`))
		})

		When("the task has no command", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "POST", "/v3/apps/the-app-guid/tasks", strings.NewReader(`{}`))
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns an unprocessable entity error", func() {
				Expect(rr.Code).To(Equal(http.StatusUnprocessableEntity))
			})
		})

		When("the app does not exist", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewNotFoundError(nil, repositories.AppResourceType))
			})

			It("returns a Not Found Error", func() {
				expectNotFoundError("App not found")
			})
		})

		When("the user cannot see the app", func() {
			BeforeEach(func() {
				appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
			})

			It("returns a Not Found error", func() {
				expectNotFoundError("App not found")
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

	Describe("GET /v3/tasks", func() {
		var (
			expectedBody string
			reqPath      string
		)

		BeforeEach(func() {
			taskRepo.ListTasksReturns([]repositories.TaskRecord{
				{
					Name:              "task-1",
					GUID:              "guid-1",
					SequenceID:        3,
					State:             "SUCCEEDED",
					MemoryMB:          1024,
					DiskMB:            1024,
					DropletGUID:       "droplet-1",
					CreationTimestamp: time.Date(2016, time.May, 4, 17, 0, 41, 0, time.UTC),
					AppGUID:           "app1",
				}, {
					Name:              "task-2",
					GUID:              "guid-2",
					SequenceID:        33,
					State:             "FAILED",
					MemoryMB:          1024,
					DiskMB:            1024,
					DropletGUID:       "droplet-2",
					CreationTimestamp: time.Date(2016, time.May, 4, 17, 0, 41, 0, time.UTC),
					AppGUID:           "app2",
				},
			}, nil)
		})

		JustBeforeEach(func() {
			expectedBody = fmt.Sprintf(`
                {
                  "pagination": {
                    "total_results": 2,
                    "total_pages": 1,
                    "first": {
                      "href": "%[1]s%[2]s"
                    },
                    "last": {
                      "href": "%[1]s%[2]s"
                    },
                    "next": null,
                    "previous": null
                  },
                  "resources": [
                    {
                      "guid": "guid-1",
                      "sequence_id": 3,
                      "name": "task-1",
                      "state": "SUCCEEDED",
                      "memory_in_mb": 1024,
                      "disk_in_mb": 1024,
                      "result": {
                        "failure_reason": null
                      },
                      "droplet_guid": "droplet-1",
                      "created_at": "2016-05-04T17:00:41Z",
                      "updated_at": "2016-05-04T17:00:41Z",
                      "relationships": {
                        "app": {
                          "data": {
                            "guid": "app1"
                          }
                        }
                      },
                      "links": {
                        "self": {
                          "href": "%[1]s/v3/tasks/guid-1"
                        },
                        "app": {
                          "href": "%[1]s/v3/apps/app1"
                        },
                        "cancel": {
                          "href": "%[1]s/v3/tasks/guid-1/actions/cancel",
                          "method": "POST"
                        },
                        "droplet": {
                          "href": "%[1]s/v3/droplets/droplet-1"
                        }
                      }
                    },
                    {
                      "guid": "guid-2",
                      "sequence_id": 33,
                      "name": "task-2",
                      "state": "FAILED",
                      "memory_in_mb": 1024,
                      "disk_in_mb": 1024,
                      "result": {
                        "failure_reason": null
                      },
                      "droplet_guid": "droplet-2",
                      "created_at": "2016-05-04T17:00:41Z",
                      "updated_at": "2016-05-04T17:00:41Z",
                      "relationships": {
                        "app": {
                          "data": {
                            "guid": "app2"
                          }
                        }
                      },
                      "links": {
                        "self": {
                          "href": "%[1]s/v3/tasks/guid-2"
                        },
                        "app": {
                          "href": "%[1]s/v3/apps/app2"
                        },
                        "cancel": {
                          "href": "%[1]s/v3/tasks/guid-2/actions/cancel",
                          "method": "POST"
                        },
                        "droplet": {
                          "href": "%[1]s/v3/droplets/droplet-2"
                        }
                      }
                    }
                  ]
                }
            `, defaultServerURL, reqPath)
		})

		Describe("GET /v3/tasks", func() {
			BeforeEach(func() {
				var err error
				req, err = http.NewRequestWithContext(ctx, "GET", "/v3/tasks", nil)
				Expect(err).NotTo(HaveOccurred())
				reqPath = "/v3/tasks"
			})

			It("returns the tasks", func() {
				Expect(rr.Code).To(Equal(http.StatusOK))
				Expect(rr.Body.String()).To(MatchJSON(expectedBody))
			})

			It("provides an empty list task message to the repository", func() {
				Expect(taskRepo.ListTasksCallCount()).To(Equal(1))
				_, _, listMsg := taskRepo.ListTasksArgsForCall(0)
				Expect(listMsg).To(Equal(repositories.ListTaskMessage{}))
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
				var err error
				req, err = http.NewRequestWithContext(ctx, "GET", "/v3/apps/the-app-guid/tasks", nil)
				Expect(err).NotTo(HaveOccurred())
				reqPath = "/v3/apps/the-app-guid/tasks"
			})

			It("returns the tasks", func() {
				Expect(rr.Code).To(Equal(http.StatusOK))
				Expect(rr.Body.String()).To(MatchJSON(expectedBody))
			})

			It("provides a list task message with the app guid to the repository", func() {
				Expect(taskRepo.ListTasksCallCount()).To(Equal(1))
				_, _, listMsg := taskRepo.ListTasksArgsForCall(0)
				Expect(listMsg.AppGUIDs).To(ConsistOf("the-app-guid"))
			})

			When("the app does not exist", func() {
				BeforeEach(func() {
					appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewNotFoundError(nil, repositories.AppResourceType))
				})

				It("returns a Not Found Error", func() {
					expectNotFoundError("App not found")
				})
			})

			When("the user cannot see the app", func() {
				BeforeEach(func() {
					appRepo.GetAppReturns(repositories.AppRecord{}, apierrors.NewForbiddenError(nil, repositories.AppResourceType))
				})

				It("returns a Not Found error", func() {
					expectNotFoundError("App not found")
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
					var err error
					req, err = http.NewRequestWithContext(ctx, "GET", "/v3/apps/the-app-guid/tasks?sequence_ids=1,2", nil)
					Expect(err).NotTo(HaveOccurred())
				})

				It("provides a list task message with the sequence ids to the repository", func() {
					Expect(taskRepo.ListTasksCallCount()).To(Equal(1))
					_, _, listMsg := taskRepo.ListTasksArgsForCall(0)
					Expect(listMsg.SequenceIDs).To(ConsistOf(int64(1), int64(2)))
				})

				When("the sequence_ids parameter cannot be parsed to ints", func() {
					BeforeEach(func() {
						var err error
						req, err = http.NewRequestWithContext(ctx, "GET", "/v3/apps/the-app-guid/tasks?sequence_ids=asdf", nil)
						Expect(err).NotTo(HaveOccurred())
					})

					It("returns a bad request error", func() {
						expectBadRequestError()
					})
				})
			})

			When("the query string contains unsupported keys", func() {
				BeforeEach(func() {
					var err error
					req, err = http.NewRequestWithContext(ctx, "GET", "/v3/apps/the-app-guid/tasks?whatever=1", nil)
					Expect(err).NotTo(HaveOccurred())
				})

				It("returns an unknown key error", func() {
					expectUnknownKeyError("The query parameter is invalid: Valid parameters are: 'sequence_ids'")
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
		var taskRecord repositories.TaskRecord

		BeforeEach(func() {
			taskRecord = repositories.TaskRecord{
				Name:              "task-name",
				GUID:              "task-guid",
				Command:           "echo hello",
				AppGUID:           "app-guid",
				DropletGUID:       "droplet-guid",
				SequenceID:        314,
				CreationTimestamp: time.Date(2022, 6, 21, 11, 11, 55, 0, time.UTC),
				MemoryMB:          123,
				DiskMB:            234,
				State:             "stateful",
			}

			taskRepo.GetTaskStub = func(context.Context, authorization.Info, string) (repositories.TaskRecord, error) {
				return taskRecord, nil
			}

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/tasks/task-guid", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("gets the correct task", func() {
			Expect(taskRepo.GetTaskCallCount()).To(Equal(1))
			_, actualAuthInfo, taskGUID := taskRepo.GetTaskArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(taskGUID).To(Equal("task-guid"))
		})

		It("returns the task", func() {
			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Body).To(MatchJSON(`{
              "name": "task-name",
              "guid": "task-guid",
              "command": "echo hello",
              "sequence_id": 314,
              "created_at": "2022-06-21T11:11:55Z",
              "updated_at": "2022-06-21T11:11:55Z",
              "memory_in_mb": 123,
              "disk_in_mb": 234,
              "droplet_guid": "droplet-guid",
              "state": "stateful",
              "relationships": {
                "app": {
                  "data": {
                    "guid": "app-guid"
                  }
                }
              },
              "result": {
                "failure_reason": null
              },
              "links": {
                "self": {
                  "href": "https://api.example.org/v3/tasks/task-guid"
                },
                "app": {
                  "href": "https://api.example.org/v3/apps/app-guid"
                },
                "droplet": {
                  "href": "https://api.example.org/v3/droplets/droplet-guid"
                },
                "cancel": {
                  "href": "https://api.example.org/v3/tasks/task-guid/actions/cancel",
                  "method": "POST"
                }
              }
            }`))
		})

		When("there FailureReason is set", func() {
			BeforeEach(func() {
				taskRecord.FailureReason = "bar"
			})

			It("propagates to the response body", func() {
				Expect(rr.Body.String()).To(ContainSubstring(`"failure_reason":"bar"`))
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
				Name:              "task-name",
				GUID:              "task-guid",
				Command:           "echo hello",
				AppGUID:           "app-guid",
				DropletGUID:       "droplet-guid",
				SequenceID:        314,
				CreationTimestamp: time.Date(2022, 6, 21, 11, 11, 55, 0, time.UTC),
				MemoryMB:          123,
				DiskMB:            234,
				State:             "stateful",
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "POST", "/v3/tasks/task-guid/actions/cancel", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("gets the task", func() {
			Expect(taskRepo.GetTaskCallCount()).To(Equal(1))
			_, actualAuthInfo, taskGUID := taskRepo.GetTaskArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(taskGUID).To(Equal("task-guid"))
		})

		When("getting the task fails with forbidden error", func() {
			BeforeEach(func() {
				taskRepo.GetTaskReturns(repositories.TaskRecord{}, apierrors.NewForbiddenError(nil, repositories.TaskResourceType))
			})

			It("returns a Not Found Error", func() {
				expectNotFoundError("Task not found")
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

		It("cancels the task", func() {
			Expect(taskRepo.CancelTaskCallCount()).To(Equal(1))
			_, actualAuthInfo, taskGUID := taskRepo.CancelTaskArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(taskGUID).To(Equal("task-guid"))
		})

		It("returns the cancelled task", func() {
			Expect(rr.Code).To(Equal(http.StatusAccepted))
			Expect(rr.Body).To(MatchJSON(`{
              "name": "task-name",
              "guid": "task-guid",
              "command": "echo hello",
              "sequence_id": 314,
              "created_at": "2022-06-21T11:11:55Z",
              "updated_at": "2022-06-21T11:11:55Z",
              "memory_in_mb": 123,
              "disk_in_mb": 234,
              "droplet_guid": "droplet-guid",
              "state": "stateful",
              "relationships": {
                "app": {
                  "data": {
                    "guid": "app-guid"
                  }
                }
              },
              "result": {
                "failure_reason": null
              },
              "links": {
                "self": {
                  "href": "https://api.example.org/v3/tasks/task-guid"
                },
                "app": {
                  "href": "https://api.example.org/v3/apps/app-guid"
                },
                "droplet": {
                  "href": "https://api.example.org/v3/droplets/droplet-guid"
                },
                "cancel": {
                  "href": "https://api.example.org/v3/tasks/task-guid/actions/cancel",
                  "method": "POST"
                }
              }
            }`))
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
				Name:              "task-name",
				GUID:              "task-guid",
				Command:           "echo hello",
				AppGUID:           "app-guid",
				DropletGUID:       "droplet-guid",
				SequenceID:        314,
				CreationTimestamp: time.Date(2022, 6, 21, 11, 11, 55, 0, time.UTC),
				MemoryMB:          123,
				DiskMB:            234,
				State:             "stateful",
			}, nil)

			var err error
			req, err = http.NewRequestWithContext(ctx, "PUT", "/v3/tasks/task-guid/cancel", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("cancels the task", func() {
			Expect(taskRepo.CancelTaskCallCount()).To(Equal(1))
			_, actualAuthInfo, taskGUID := taskRepo.CancelTaskArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(taskGUID).To(Equal("task-guid"))
		})
	})
})
