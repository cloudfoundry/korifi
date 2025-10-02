package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("Build", func() {
	Describe("BuildCreate", func() {
		var createPayload payloads.BuildCreate

		BeforeEach(func() {
			createPayload = payloads.BuildCreate{
				Package: &payloads.RelationshipData{
					GUID: "some-build-guid",
				},
				Lifecycle: &payloads.Lifecycle{
					Type: "buildpack",
					Data: payloads.LifecycleData{
						Buildpacks: []string{"bp1"},
						Stack:      "stack",
					},
				},
			}
		})

		Describe("Decode", func() {
			var (
				decodedBuildPayload *payloads.BuildCreate
				validatorErr        error
			)

			BeforeEach(func() {
				decodedBuildPayload = new(payloads.BuildCreate)
			})

			JustBeforeEach(func() {
				validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(createPayload), decodedBuildPayload)
			})

			It("succeeds", func() {
				Expect(validatorErr).NotTo(HaveOccurred())
				Expect(decodedBuildPayload).To(gstruct.PointTo(Equal(createPayload)))
			})

			When("package is not provided", func() {
				BeforeEach(func() {
					createPayload.Package = nil
				})

				It("says package is required", func() {
					expectUnprocessableEntityError(validatorErr, "package cannot be blank")
				})
			})

			When("package guid is empty", func() {
				BeforeEach(func() {
					createPayload.Package.GUID = ""
				})

				It("says guid is required", func() {
					expectUnprocessableEntityError(validatorErr, "package.guid cannot be blank")
				})
			})

			When("the metadata annotations is not empty", func() {
				BeforeEach(func() {
					createPayload.Metadata.Annotations = map[string]string{
						"foo": "bar",
					}
				})

				It("says labels and annotations are not supported", func() {
					expectUnprocessableEntityError(validatorErr, "metadata.annotations must be blank")
				})
			})

			When("the metadata labels is not empty", func() {
				BeforeEach(func() {
					createPayload.Metadata.Labels = map[string]string{
						"foo": "bar",
					}
				})

				It("says labels and annotations are not supported", func() {
					expectUnprocessableEntityError(validatorErr, "metadata.labels must be blank")
				})
			})

			When("the lifecycle is invalid", func() {
				BeforeEach(func() {
					createPayload.Lifecycle = &payloads.Lifecycle{
						Type: "invalid",
					}
				})

				It("says lifecycle is invalid", func() {
					expectUnprocessableEntityError(validatorErr, "lifecycle.type value must be one of: buildpack, docker")
				})
			})
		})

		Describe("ToMessage", func() {
			var createMessage repositories.CreateBuildMessage

			JustBeforeEach(func() {
				createMessage = createPayload.ToMessage(repositories.AppRecord{
					GUID:      "guid",
					SpaceGUID: "space-guid",
					Lifecycle: repositories.Lifecycle{
						Type: "docker",
					},
				})
			})

			It("translates to create build repo message", func() {
				Expect(createMessage).To(Equal(repositories.CreateBuildMessage{
					AppGUID:         "guid",
					PackageGUID:     "some-build-guid",
					SpaceGUID:       "space-guid",
					StagingMemoryMB: payloads.DefaultLifecycleConfig.StagingMemoryMB,
					Lifecycle: repositories.Lifecycle{
						Type: "buildpack",
						Data: repositories.LifecycleData{
							Buildpacks: []string{"bp1"},
							Stack:      "stack",
						},
					},
				}))
			})

			When("the create message lifecycle data is not specified", func() {
				BeforeEach(func() {
					createPayload.Lifecycle = nil
				})

				It("defaults the lifecycle to the app's one", func() {
					Expect(createMessage.Lifecycle).To(Equal(repositories.Lifecycle{
						Type: "docker",
					}))
				})
			})
		})
	})

	Describe("BuildList", func() {
		Describe("Validation", func() {
			DescribeTable("valid query",
				func(query string, expectedBuildList payloads.BuildList) {
					actualBuildList, decodeErr := decodeQuery[payloads.BuildList](query)

					Expect(decodeErr).NotTo(HaveOccurred())
					Expect(*actualBuildList).To(Equal(expectedBuildList))
				},

				Entry("states", "states=s1,s2", payloads.BuildList{States: "s1,s2"}),
				Entry("app_guids", "app_guids=guid1,guid2", payloads.BuildList{AppGUIDs: "guid1,guid2"}),
				Entry("package_guids", "package_guids=pg1,pg2", payloads.BuildList{PackageGUIDs: "pg1,pg2"}),
				Entry("order_by created_at", "order_by=created_at", payloads.BuildList{OrderBy: "created_at"}),
				Entry("order_by -created_at", "order_by=-created_at", payloads.BuildList{OrderBy: "-created_at"}),
				Entry("order_by updated_at", "order_by=updated_at", payloads.BuildList{OrderBy: "updated_at"}),
				Entry("order_by -updated_at", "order_by=-updated_at", payloads.BuildList{OrderBy: "-updated_at"}),
				Entry("page=3", "page=3", payloads.BuildList{Pagination: payloads.Pagination{Page: "3"}}),
			)

			DescribeTable("invalid query",
				func(query string, expectedErrMsg string) {
					_, decodeErr := decodeQuery[payloads.BuildList](query)
					Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
				},
				Entry("invalid order_by", "order_by=foo", "value must be one of"),
				Entry("per_page is not a number", "per_page=foo", "value must be an integer"),
			)
		})

		Describe("ToMessage", func() {
			var (
				buildList payloads.BuildList
				message   repositories.ListBuildsMessage
			)

			BeforeEach(func() {
				buildList = payloads.BuildList{
					PackageGUIDs: "pg1,pg2",
					AppGUIDs:     "ag1,ag2",
					States:       "s1,s2",
					OrderBy:      "created_at",
					Pagination: payloads.Pagination{
						PerPage: "10",
						Page:    "4",
					},
				}
			})

			JustBeforeEach(func() {
				message = buildList.ToMessage()
			})

			It("translates to repository message", func() {
				Expect(message).To(Equal(repositories.ListBuildsMessage{
					PackageGUIDs: []string{"pg1", "pg2"},
					AppGUIDs:     []string{"ag1", "ag2"},
					States:       []string{"s1", "s2"},
					OrderBy:      "created_at",
					Pagination: repositories.Pagination{
						PerPage: 10,
						Page:    4,
					},
				}))
			})
		})
	})
})
