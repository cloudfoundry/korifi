package e2e_test

import (
	"net/http"

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
				SetResult(&createdTask).
				Post("/v3/apps/{appGUID}/tasks")
			Expect(err).NotTo(HaveOccurred())
		})

		When("the user has space developer role in the space", func() {
			BeforeEach(func() {
				createSpaceRole("space_developer", certUserName, spaceGUID)
			})

			It("succeeds", func() {
				Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
				Expect(createdTask.State).ToNot(BeEmpty())
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
			Expect(resp).To(HaveRestyStatusCode(http.StatusCreated))
			Expect(err).NotTo(HaveOccurred())
		})

		It("succeeds", func() {
			Eventually(func(g Gomega) {
				var task taskResource
				getResp, err := certClient.R().
					SetPathParam("taskGUID", createdTask.GUID).
					SetResult(&task).
					Get("/v3/tasks/{taskGUID}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(getResp).To(HaveRestyStatusCode(http.StatusOK))
				g.Expect(task.GUID).To(Equal(createdTask.GUID))
				g.Expect(task.State).To(Equal("SUCCEEDED"))
			}).Should(Succeed())
		})
	})

	Describe("Listing tasks", func() {
		var (
			list  resourceList
			guids []string
		)

		BeforeEach(func() {
			createSpaceRole("space_developer", certUserName, spaceGUID)

			guids = nil
			var err error
			for i := 0; i < 2; i++ {
				resp, err = certClient.R().
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
			listResp, err := certClient.R().
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
			list  resourceList
			guids []string
		)

		BeforeEach(func() {
			createSpaceRole("space_developer", certUserName, spaceGUID)

			guids = nil
			var err error

			for i := 0; i < 2; i++ {
				resp, err = certClient.R().
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
			listResp, err := certClient.R().
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
})
