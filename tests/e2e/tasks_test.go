package e2e_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/tests/helpers"
	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Tasks", func() {
	var (
		spaceGUID   string
		appGUID     string
		resp        *resty.Response
		createdTask taskResource
	)

	BeforeEach(func() {
		spaceGUID = createSpace(generateGUID("space"), commonTestOrgGUID)
		appGUID, _ = pushTestApp(spaceGUID, defaultAppBitsFile)
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	eventuallyTaskShouldHaveState := func(taskGUID, expectedState string) {
		Eventually(func(g Gomega) {
			var task taskResource
			getResp, err := adminClient.R().
				SetPathParam("taskGUID", taskGUID).
				SetResult(&task).
				Get("/v3/tasks/{taskGUID}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(getResp).To(HaveRestyStatusCode(http.StatusOK))
			g.Expect(task.GUID).To(Equal(taskGUID))
			g.Expect(task.State).To(Equal(expectedState))
		}).Should(Succeed())
	}

	Describe("Create a task", func() {
		var command string

		BeforeEach(func() {
			command = "echo hello"
		})

		JustBeforeEach(func() {
			var err error
			resp, err = adminClient.R().
				SetBody(taskResource{
					Command: command,
				}).
				SetPathParam("appGUID", appGUID).
				SetResult(&createdTask).
				Post("/v3/apps/{appGUID}/tasks")
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(createdTask.State).ToNot(BeEmpty())
		})

		When("the task fails", func() {
			BeforeEach(func() {
				command = "false"
			})

			It("is eventually marked as failed", func() {
				eventuallyTaskShouldHaveState(createdTask.GUID, "FAILED")
			})
		})
	})

	Describe("Get a task", func() {
		BeforeEach(func() {
			var err error

			resp, err = adminClient.R().
				SetBody(taskResource{
					Command: "/bin/sh -c 'echo hello world'",
				}).
				SetPathParam("appGUID", appGUID).
				SetResult(&createdTask).
				Post("/v3/apps/{appGUID}/tasks")
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds and is cleaned up after its TTL", func() {
			eventuallyTaskShouldHaveState(createdTask.GUID, "SUCCEEDED")

			Eventually(func(g Gomega) {
				var task taskResource
				getResp, err := adminClient.R().
					SetPathParam("taskGUID", createdTask.GUID).
					SetResult(&task).
					Get("/v3/tasks/{taskGUID}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(getResp).To(HaveRestyStatusCode(http.StatusNotFound))
			}).Should(Succeed())
		})
	})

	Describe("Listing tasks", func() {
		var (
			list  resourceList[resource]
			guids []string
		)

		BeforeEach(func() {
			guids = nil
			var err error
			for i := 0; i < 2; i++ {
				resp, err = adminClient.R().
					SetBody(taskResource{
						Command: "echo hello",
					}).
					SetPathParam("appGUID", appGUID).
					SetResult(&createdTask).
					Post("/v3/apps/{appGUID}/tasks")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
				guids = append(guids, createdTask.GUID)
			}
		})

		JustBeforeEach(func() {
			listResp, err := adminClient.R().
				SetResult(&list).
				Get("/v3/tasks")
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp).To(HaveRestyStatusCode(http.StatusOK))
		})

		It("lists the 2 tasks", func() {
			Expect(list.Resources).To(ContainElements(
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(guids[0])}),
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(guids[1])}),
			))
		})
	})

	Describe("List app's tasks", func() {
		var (
			list  resourceList[resource]
			guids []string
		)

		BeforeEach(func() {
			guids = nil
			var err error

			for i := 0; i < 2; i++ {
				resp, err = adminClient.R().
					SetBody(taskResource{
						Command: "echo hello",
					}).
					SetPathParam("appGUID", appGUID).
					SetResult(&createdTask).
					Post("/v3/apps/{appGUID}/tasks")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
				guids = append(guids, createdTask.GUID)
			}
		})

		JustBeforeEach(func() {
			listResp, err := adminClient.R().
				SetPathParam("appGUID", appGUID).
				SetResult(&list).
				Get("/v3/apps/{appGUID}/tasks")
			Expect(err).NotTo(HaveOccurred())
			Expect(listResp).To(HaveRestyStatusCode(http.StatusOK))
		})

		It("lists the 2 tasks", func() {
			Expect(list.Resources).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(guids[0])}),
				MatchFields(IgnoreExtras, Fields{"GUID": Equal(guids[1])}),
			))
		})
	})

	Describe("cancelling a task", func() {
		var (
			returnedTask   taskResource
			cancelResp     *resty.Response
			partialRequest helpers.Request
			command        string
		)

		BeforeEach(func() {
			command = "sleep 100"
		})

		JustBeforeEach(func() {
			resp, err := adminClient.R().
				SetBody(taskResource{
					Command: command,
				}).
				SetPathParam("appGUID", appGUID).
				SetResult(&createdTask).
				Post("/v3/apps/{appGUID}/tasks")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))

			partialRequest = adminClient.R().
				SetPathParam("taskGUID", createdTask.GUID).
				SetResult(&returnedTask)
		})

		When("using the new API", func() {
			JustBeforeEach(func() {
				var err error
				cancelResp, err = partialRequest.Post("/v3/tasks/{taskGUID}/actions/cancel")
				Expect(err).NotTo(HaveOccurred())
			})

			It("cancels the task", func() {
				Expect(cancelResp).To(HaveRestyStatusCode(http.StatusAccepted))
				Expect(returnedTask.GUID).To(Equal(createdTask.GUID))
				eventuallyTaskShouldHaveState(createdTask.GUID, "FAILED")
			})
		})

		When("the task has completed", func() {
			BeforeEach(func() {
				command = "true"
			})

			JustBeforeEach(func() {
				eventuallyTaskShouldHaveState(createdTask.GUID, "SUCCEEDED")
				var err error
				cancelResp, err = partialRequest.Post("/v3/tasks/{taskGUID}/actions/cancel")
				Expect(err).NotTo(HaveOccurred())
			})

			It("is not possible to cancel the task", func() {
				Expect(cancelResp).To(HaveRestyStatusCode(http.StatusUnprocessableEntity))
				Expect(cancelResp).To(HaveRestyBody(ContainSubstring("CF-UnprocessableEntity")))
				Expect(cancelResp).To(HaveRestyBody(ContainSubstring("Task state is SUCCEEDED and therefore cannot be canceled")))
			})
		})

		When("using the deprecated API", func() {
			JustBeforeEach(func() {
				var err error
				cancelResp, err = partialRequest.Put("/v3/tasks/{taskGUID}/cancel")
				Expect(err).NotTo(HaveOccurred())
			})

			It("cancels the task", func() {
				Expect(cancelResp).To(HaveRestyStatusCode(http.StatusAccepted))
				Expect(returnedTask.GUID).To(Equal(createdTask.GUID))
				eventuallyTaskShouldHaveState(createdTask.GUID, "FAILED")
			})
		})
	})
})
