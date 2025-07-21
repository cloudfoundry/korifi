package repositories_test

import (
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("UserRepository", func() {
	var userRepo *repositories.UserRepository

	BeforeEach(func() {
		userRepo = repositories.NewUserRepository()
	})

	Describe("ListUsers", func() {
		var (
			users   repositories.ListResult[repositories.UserRecord]
			message repositories.ListUsersMessage
		)

		BeforeEach(func() {
			message = repositories.ListUsersMessage{
				Names: []string{"user-1", "user-2"},
			}
		})

		JustBeforeEach(func() {
			var err error
			users, err = userRepo.ListUsers(ctx, authInfo, message)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns synthetic users", func() {
			Expect(users.Records).To(ConsistOf(
				repositories.UserRecord{GUID: "user-1", Name: "user-1"},
				repositories.UserRecord{GUID: "user-2", Name: "user-2"},
			))
			Expect(users.PageInfo).To(Equal(descriptors.PageInfo{
				TotalResults: 2,
				TotalPages:   1,
				PageNumber:   1,
				PageSize:     2,
			}))
		})

		When("paging is requested", func() {
			BeforeEach(func() {
				message.Pagination = repositories.Pagination{
					PerPage: 1,
					Page:    2,
				}
			})

			It("returns users page", func() {
				Expect(users.Records).To(ConsistOf(
					repositories.UserRecord{GUID: "user-2", Name: "user-2"},
				))
				Expect(users.PageInfo).To(Equal(descriptors.PageInfo{
					TotalResults: 2,
					TotalPages:   2,
					PageNumber:   2,
					PageSize:     1,
				}))
			})
		})

		When("no user names are specified in the message", func() {
			BeforeEach(func() {
				message.Names = nil
			})

			It("returns an empty result", func() {
				Expect(users.Records).To(BeEmpty())
			})
		})
	})
})
