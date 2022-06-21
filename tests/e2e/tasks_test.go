package e2e_test

import (
	"net/http"

	"github.com/go-resty/resty/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tasks", func() {
	var (
		spaceGUID string
		appGUID   string
		resp      *resty.Response
	)

	BeforeEach(func() {
		spaceGUID = createSpace(generateGUID("space"), commonTestOrgGUID)
		appGUID = pushTestApp(spaceGUID, appBitsFile)
	})

	AfterEach(func() {
		deleteSpace(spaceGUID)
	})

	Describe("Create a task", func() {
		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetBody(taskResource{
					Command: "echo hello",
				}).
				SetPathParam("appGUID", appGUID).
				Post("/v3/apps/{appGUID}/tasks")
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user has space developer role in the space", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			It("succeeds", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			})
		})

		When("the user cannot create tasks in the space", func() {
			BeforeEach(func() {
				createSpaceRole("space_manager", certUserName, spaceGUID)
			})

			It("fails", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusForbidden))
				Expect(resp).To(HaveRestyBody(ContainSubstring("CF-NotAuthorized")))
			})
		})
	})

	Describe("Get a task", func() {
		var (
			createdTask resource
			task        resource
		)

		BeforeEach(func() {
			var err error
			createSpaceRole("space_developer", certUserName, spaceGUID)
			resp, err = certClient.R().
				SetBody(taskResource{
					Command: "echo hello",
				}).
				SetPathParam("appGUID", appGUID).
				SetResult(&createdTask).
				Post("/v3/apps/{appGUID}/tasks")
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			var err error
			resp, err = certClient.R().
				SetPathParam("taskGUID", createdTask.GUID).
				SetResult(&task).
				Get("/v3/tasks/{taskGUID}")
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds", func() {
			Expect(resp).To(HaveRestyStatusCode(http.StatusOK))
			Expect(task.GUID).To(Equal(createdTask.GUID))
		})
	})
})
