package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"

	. "code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("Space", func() {
	var (
		apiHandler       *handlers.Space
		spaceRepo        *fake.CFSpaceRepository
		requestValidator *fake.RequestValidator
		requestMethod    string
		requestPath      string
	)

	BeforeEach(func() {
		requestPath = "/v3/spaces"

		requestValidator = new(fake.RequestValidator)
		spaceRepo = new(fake.CFSpaceRepository)
		spaceRepo.GetSpaceReturns(repositories.SpaceRecord{
			Name:             "the-space",
			GUID:             "the-space-guid",
			OrganizationGUID: "the-org-guid",
		}, nil)

		apiHandler = handlers.NewSpace(
			*serverURL,
			spaceRepo,
			requestValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestPath, strings.NewReader("the-json-body"))
		Expect(err).NotTo(HaveOccurred())

		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("Create Space", func() {
		BeforeEach(func() {
			requestMethod = http.MethodPost

			spaceRepo.CreateSpaceReturns(repositories.SpaceRecord{
				Name:             "the-space",
				GUID:             "the-space-guid",
				OrganizationGUID: "the-org-guid",
			}, nil)

			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.SpaceCreate{
				Name: "the-space",
				Relationships: &payloads.SpaceRelationships{
					Org: &payloads.Relationship{
						Data: &payloads.RelationshipData{
							GUID: "org-guid",
						},
					},
				},
			})
		})

		It("creates the space", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

			Expect(spaceRepo.CreateSpaceCallCount()).To(Equal(1))
			_, info, spaceRecord := spaceRepo.CreateSpaceArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(spaceRecord.Name).To(Equal("the-space"))

			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "the-space-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/spaces/the-space-guid"),
			)))
		})

		When("the parent org does not exist (and the repo returns a not found error)", func() {
			BeforeEach(func() {
				spaceRepo.CreateSpaceReturns(repositories.SpaceRecord{}, apierrors.NewNotFoundError(errors.New("nope"), repositories.OrgResourceType))
			})

			It("returns an unauthorised error", func() {
				expectUnprocessableEntityError("Invalid organization. Ensure the organization exists and you have access to it.")
			})
		})

		When("creating the space fails", func() {
			BeforeEach(func() {
				spaceRepo.CreateSpaceReturns(repositories.SpaceRecord{}, errors.New("nope"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the request is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("Listing Spaces", func() {
		BeforeEach(func() {
			requestMethod = http.MethodGet
			requestPath += "?foo=bar"
			spaceRepo.ListSpacesReturns([]repositories.SpaceRecord{
				{
					Name:             "test-space-1",
					GUID:             "test-space-1-guid",
					OrganizationGUID: "test-org-1-guid",
				},
				{
					Name:             "test-space-2",
					GUID:             "test-space-2-guid",
					OrganizationGUID: "test-org-2-guid",
				},
			}, nil)
			requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.SpaceList{})
		})

		It("returns a list of spaces", func() {
			Expect(requestValidator.DecodeAndValidateURLValuesCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateURLValuesArgsForCall(0)
			Expect(actualReq.URL.String()).To(HaveSuffix(requestPath))

			Expect(spaceRepo.ListSpacesCallCount()).To(Equal(1))
			_, info, message := spaceRepo.ListSpacesArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(message.OrganizationGUIDs).To(BeEmpty())
			Expect(message.Names).To(BeEmpty())

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.pagination.first.href", "https://api.example.org/v3/spaces?foo=bar"),
				MatchJSONPath("$.resources", HaveLen(2)),
				MatchJSONPath("$.resources[0].guid", "test-space-1-guid"),
				MatchJSONPath("$.resources[0].links.self.href", "https://api.example.org/v3/spaces/test-space-1-guid"),
				MatchJSONPath("$.resources[1].guid", "test-space-2-guid"),
			)))
		})

		When("fetching the spaces fails", func() {
			BeforeEach(func() {
				spaceRepo.ListSpacesReturns(nil, errors.New("boom!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("organization_guids are provided as a comma-separated list", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.SpaceList{
					OrganizationGUIDs: "org1,org2",
				})
			})

			It("filters spaces by them", func() {
				Expect(spaceRepo.ListSpacesCallCount()).To(Equal(1))
				_, info, message := spaceRepo.ListSpacesArgsForCall(0)
				Expect(info).To(Equal(authInfo))
				Expect(message.OrganizationGUIDs).To(ConsistOf("org1", "org2"))
				Expect(message.Names).To(BeEmpty())
			})
		})

		When("names are provided as a comma-separated list", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateURLValuesStub = decodeAndValidateURLValuesStub(&payloads.SpaceList{
					Names: "name1,name2",
				})
			})

			It("filters spaces by them", func() {
				Expect(spaceRepo.ListSpacesCallCount()).To(Equal(1))
				_, info, message := spaceRepo.ListSpacesArgsForCall(0)
				Expect(info).To(Equal(authInfo))
				Expect(message.OrganizationGUIDs).To(BeEmpty())
				Expect(message.Names).To(ConsistOf("name1", "name2"))
			})
		})
	})

	Describe("Deleting a Space", func() {
		BeforeEach(func() {
			requestMethod = http.MethodDelete
			requestPath += "/the-space-guid"
		})

		It("deletes the space", func() {
			Expect(spaceRepo.GetSpaceCallCount()).To(Equal(1))
			_, info, actualSpaceGUID := spaceRepo.GetSpaceArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(actualSpaceGUID).To(Equal("the-space-guid"))

			Expect(spaceRepo.DeleteSpaceCallCount()).To(Equal(1))
			_, info, message := spaceRepo.DeleteSpaceArgsForCall(0)
			Expect(info).To(Equal(authInfo))
			Expect(message).To(Equal(repositories.DeleteSpaceMessage{
				GUID:             "the-space-guid",
				OrganizationGUID: "the-org-guid",
			}))

			Expect(rr).To(HaveHTTPStatus(http.StatusAccepted))
			Expect(rr).To(HaveHTTPHeaderWithValue("Location", "https://api.example.org/v3/jobs/space.delete~the-space-guid"))
		})

		When("fetching the space errors", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("deleting the space errors", func() {
			BeforeEach(func() {
				spaceRepo.DeleteSpaceReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("Updating Space", func() {
		BeforeEach(func() {
			requestMethod = http.MethodPatch
			requestPath += "/the-space-guid"

			spaceRepo.PatchSpaceMetadataReturns(repositories.SpaceRecord{
				Name:             "the-space",
				GUID:             "the-space-guid",
				OrganizationGUID: "the-org-guid",
			}, nil)

			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payloads.SpacePatch{
				Metadata: payloads.MetadataPatch{
					Annotations: map[string]*string{
						"hello":                       tools.PtrTo("there"),
						"foo.example.com/lorem-ipsum": tools.PtrTo("Lorem ipsum."),
					},
					Labels: map[string]*string{
						"env":                           tools.PtrTo("production"),
						"foo.example.com/my-identifier": tools.PtrTo("aruba"),
					},
				},
			})
		})

		It("updates the space", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))

			Expect(spaceRepo.PatchSpaceMetadataCallCount()).To(Equal(1))
			_, _, msg := spaceRepo.PatchSpaceMetadataArgsForCall(0)
			Expect(msg.GUID).To(Equal("the-space-guid"))
			Expect(msg.OrgGUID).To(Equal("the-org-guid"))
			Expect(msg.Annotations).To(HaveKeyWithValue("hello", PointTo(Equal("there"))))
			Expect(msg.Annotations).To(HaveKeyWithValue("foo.example.com/lorem-ipsum", PointTo(Equal("Lorem ipsum."))))
			Expect(msg.Labels).To(HaveKeyWithValue("env", PointTo(Equal("production"))))
			Expect(msg.Labels).To(HaveKeyWithValue("foo.example.com/my-identifier", PointTo(Equal("aruba"))))

			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "the-space-guid"),
				MatchJSONPath("$.links.self.href", "https://api.example.org/v3/spaces/the-space-guid"),
			)))
		})

		When("the user doesn't have permission to get the org", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, apierrors.NewForbiddenError(nil, repositories.SpaceResourceType))
			})

			It("returns a not found error and does not try updating the space", func() {
				expectNotFoundError(repositories.SpaceResourceType)
				Expect(spaceRepo.PatchSpaceMetadataCallCount()).To(Equal(0))
			})
		})

		When("fetching the org errors", func() {
			BeforeEach(func() {
				spaceRepo.GetSpaceReturns(repositories.SpaceRecord{}, errors.New("boom"))
			})

			It("returns an error and does not try updating the space", func() {
				expectUnknownError()
				Expect(spaceRepo.PatchSpaceMetadataCallCount()).To(Equal(0))
			})
		})

		When("patching the org errors", func() {
			BeforeEach(func() {
				spaceRepo.PatchSpaceMetadataReturns(repositories.SpaceRecord{}, errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the request is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(errors.New("boom"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})
	})
})
