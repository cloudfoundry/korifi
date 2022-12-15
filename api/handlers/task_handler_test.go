package handlers_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TaskHandler", func() {
	var (
		req         *http.Request
		taskRepo    *fake.CFTaskRepository
		taskHandler http.Handler
	)

	BeforeEach(func() {
		taskRepo = new(fake.CFTaskRepository)
		decoderValidator, err := handlers.NewDefaultDecoderValidator()
		Expect(err).NotTo(HaveOccurred())

		taskHandler = handlers.NewTaskHandler(*serverURL, taskRepo, decoderValidator)
	})

	JustBeforeEach(func() {
		taskHandler.ServeHTTP(rr, req)
	})

	Describe("GET /v3/tasks", func() {
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

			var err error
			req, err = http.NewRequestWithContext(ctx, "GET", "/v3/tasks", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the tasks", func() {
			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(rr.Body.String()).To(MatchJSON(fmt.Sprintf(`
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
				`, defaultServerURL, "/v3/tasks")))
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
