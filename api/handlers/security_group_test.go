package handlers_test

import (
	"errors"
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	. "code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SecurityGroup", func() {
	var (
		requestMethod     string
		requestPath       string
		requestBody       string
		securityGroupRepo *fake.CFSecurityGroupRepository
		spaceRepo         *fake.CFSpaceRepository
		requestValidator  *fake.RequestValidator
	)

	BeforeEach(func() {
		securityGroupRepo = new(fake.CFSecurityGroupRepository)
		spaceRepo = new(fake.CFSpaceRepository)
		requestValidator = new(fake.RequestValidator)

		apiHandler := NewSecurityGroup(
			*serverURL,
			securityGroupRepo,
			spaceRepo,
			requestValidator,
		)
		routerBuilder.LoadRoutes(apiHandler)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestPath, strings.NewReader(requestBody))
		Expect(err).NotTo(HaveOccurred())

		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("GET /v3/security_groups/{guid}", func() {
		BeforeEach(func() {
			requestMethod = http.MethodGet
			requestPath = "/v3/security_groups/test-guid"
			requestBody = ""

			securityGroupRepo.GetSecurityGroupReturns(repositories.SecurityGroupRecord{
				GUID: "test-guid",
				Name: "test-security-group",
			}, nil)
		})

		It("returns the security group", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "test-guid"),
				MatchJSONPath("$.name", "test-security-group"),
			)))
		})

		When("the security group is not found", func() {
			BeforeEach(func() {
				securityGroupRepo.GetSecurityGroupReturns(repositories.SecurityGroupRecord{}, apierrors.NewNotFoundError(nil, "SecurityGroup"))
			})

			It("returns a not found error", func() {
				expectNotFoundError("SecurityGroup")
			})
		})

		When("the repository returns an error", func() {
			BeforeEach(func() {
				securityGroupRepo.GetSecurityGroupReturns(repositories.SecurityGroupRecord{}, errors.New("boom"))
			})

			It("returns an unknown error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("POST /v3/security_groups", func() {
		var payload payloads.SecurityGroupCreate

		BeforeEach(func() {
			requestMethod = http.MethodPost
			requestPath = "/v3/security_groups"
			requestBody = "the-json-body"

			payload = payloads.SecurityGroupCreate{
				DisplayName: "test-security-group",
				Rules: []payloads.SecurityGroupRule{
					{
						Protocol:    korifiv1alpha1.ProtocolTCP,
						Ports:       "80",
						Destination: "192.168.1.1",
					},
				},
				Relationships: payloads.SecurityGroupRelationships{
					RunningSpaces: payloads.ToManyRelationship{
						Data: []payloads.RelationshipData{
							{
								GUID: "space1",
							},
						},
					},
					StagingSpaces: payloads.ToManyRelationship{
						Data: []payloads.RelationshipData{
							{
								GUID: "space2",
							},
						},
					},
				},
			}
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payload)

			securityGroupRepo.CreateSecurityGroupReturns(repositories.SecurityGroupRecord{
				GUID: "test-guid",
				Name: "test-security-group",
				Rules: []repositories.SecurityGroupRule{
					{
						Protocol:    korifiv1alpha1.ProtocolTCP,
						Ports:       "80",
						Destination: "192.168.1.1",
					},
				},
				RunningSpaces: []string{"space1"},
				StagingSpaces: []string{"space2"},
			}, nil)

			spaceRepo.ListSpacesReturns([]repositories.SpaceRecord{
				{
					Name: "space1",
				},
				{
					Name: "space2",
				},
			}, nil)
		})

		It("validates the request", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))
		})

		It("creates a security group with a rule", func() {
			Expect(securityGroupRepo.CreateSecurityGroupCallCount()).To(Equal(1))
			Expect(spaceRepo.ListSpacesCallCount()).To(Equal(1))

			_, actualAuthInfo, createSecurityGroupMessage := securityGroupRepo.CreateSecurityGroupArgsForCall(0)
			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(createSecurityGroupMessage.DisplayName).To(Equal("test-security-group"))
			Expect(createSecurityGroupMessage.Rules).To(Equal([]repositories.SecurityGroupRule{
				{
					Protocol:    korifiv1alpha1.ProtocolTCP,
					Ports:       "80",
					Destination: "192.168.1.1",
				},
			}))

			_, _, listSpacesMessage := spaceRepo.ListSpacesArgsForCall(0)
			Expect(listSpacesMessage.GUIDs).To(ConsistOf("space1", "space2"))

			Expect(rr).To(HaveHTTPStatus(http.StatusCreated))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "test-guid"),
				MatchJSONPath("$.name", "test-security-group"),
				MatchJSONPath("$.rules[0].protocol", "tcp"),
				MatchJSONPath("$.rules[0].ports", "80"),
				MatchJSONPath("$.rules[0].destination", "192.168.1.1"),
				MatchJSONPath("$.relationships.running_spaces.data[0].guid", "space1"),
				MatchJSONPath("$.relationships.staging_spaces.data[0].guid", "space2"),
			)))
		})

		When("a space in the security group does not exist", func() {
			BeforeEach(func() {
				spaceRepo.ListSpacesReturns([]repositories.SpaceRecord{}, nil)
			})

			It("returns a not found error", func() {
				expectUnprocessableEntityError("Space does not exist, or you do not have access")
			})
		})

		When("fetching the spaces returns an error", func() {
			BeforeEach(func() {
				spaceRepo.ListSpacesReturns(nil, errors.New("boom!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the request body is not valid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(apierrors.NewUnprocessableEntityError(nil, "nope"))
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("nope")
			})
		})

		When("the repository returns an error", func() {
			BeforeEach(func() {
				securityGroupRepo.CreateSecurityGroupReturns(repositories.SecurityGroupRecord{}, errors.New("repo-error"))
			})

			It("returns an unknown error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("POST /v3/security_groups/{guid}/relationships/running_spaces", func() {
		var payload payloads.SecurityGroupBind

		BeforeEach(func() {
			requestMethod = http.MethodPost
			requestPath = "/v3/security_groups/test-guid/relationships/running_spaces"
			requestBody = "the-json-body"

			payload = payloads.SecurityGroupBind{
				Data: []payloads.RelationshipData{{GUID: "space1"}},
			}
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payload)

			securityGroupRepo.GetSecurityGroupReturns(repositories.SecurityGroupRecord{
				GUID: "test-guid",
				Name: "test-security-group",
				Rules: []repositories.SecurityGroupRule{
					{
						Protocol:    korifiv1alpha1.ProtocolTCP,
						Ports:       "80",
						Destination: "192.168.1.1",
					},
				},
			}, nil)

			spaceRepo.ListSpacesReturns([]repositories.SpaceRecord{
				{GUID: "space1"},
			}, nil)

			securityGroupRepo.BindSecurityGroupReturns(repositories.SecurityGroupRecord{
				GUID: "test-guid",
				Name: "test-security-group",
				Rules: []repositories.SecurityGroupRule{
					{
						Protocol:    korifiv1alpha1.ProtocolTCP,
						Ports:       "80",
						Destination: "192.168.1.1",
					},
				},
				RunningSpaces: []string{"space1"},
			}, nil)
		})

		It("validates the request", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))
		})

		It("binds running spaces to the security group", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(securityGroupRepo.GetSecurityGroupCallCount()).To(Equal(1))
			_, actualAuthInfo, guid := securityGroupRepo.GetSecurityGroupArgsForCall(0)

			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(guid).To(Equal("test-guid"))

			Expect(spaceRepo.ListSpacesCallCount()).To(Equal(1))
			_, _, listMessage := spaceRepo.ListSpacesArgsForCall(0)
			Expect(listMessage.GUIDs).To(ConsistOf("space1"))

			Expect(securityGroupRepo.BindSecurityGroupCallCount()).To(Equal(1))
			_, _, bindMessage := securityGroupRepo.BindSecurityGroupArgsForCall(0)
			Expect(bindMessage.GUID).To(Equal("test-guid"))
			Expect(bindMessage.Spaces).To(ConsistOf("space1"))
		})

		When("the payload is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(errors.New("validation-error"))
			})

			It("returns a validation error", func() {
				expectUnknownError()
			})
		})

		When("the security group does not exist", func() {
			BeforeEach(func() {
				securityGroupRepo.GetSecurityGroupReturns(repositories.SecurityGroupRecord{}, apierrors.NewNotFoundError(nil, "SecurityGroup"))
			})

			It("returns a 404 not found error", func() {
				expectNotFoundError("SecurityGroup")
			})
		})

		When("the spaces does not exist", func() {
			BeforeEach(func() {
				spaceRepo.ListSpacesReturns([]repositories.SpaceRecord{}, nil)
			})

			It("returns an  error", func() {
				expectUnprocessableEntityError("Space does not exist, or you do not have access.")
			})
		})

		When("fetching the spaces returns an error", func() {
			BeforeEach(func() {
				spaceRepo.ListSpacesReturns(nil, errors.New("boom!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the repository returns an error", func() {
			BeforeEach(func() {
				securityGroupRepo.BindSecurityGroupReturns(repositories.SecurityGroupRecord{}, errors.New("boom"))
			})

			It("returns an unknown error", func() {
				expectUnknownError()
			})
		})
	})

	Describe("POST /v3/security_groups/{guid}/relationships/staging_spaces", func() {
		var payload payloads.SecurityGroupBind

		BeforeEach(func() {
			requestMethod = http.MethodPost
			requestPath = "/v3/security_groups/test-guid/relationships/staging_spaces"
			requestBody = "the-json-body"

			payload = payloads.SecurityGroupBind{
				Data: []payloads.RelationshipData{{GUID: "space1"}},
			}
			requestValidator.DecodeAndValidateJSONPayloadStub = decodeAndValidatePayloadStub(&payload)

			securityGroupRepo.GetSecurityGroupReturns(repositories.SecurityGroupRecord{
				GUID: "test-guid",
				Name: "test-security-group",
				Rules: []repositories.SecurityGroupRule{
					{
						Protocol:    korifiv1alpha1.ProtocolTCP,
						Ports:       "80",
						Destination: "192.168.1.1",
					},
				},
			}, nil)

			spaceRepo.ListSpacesReturns([]repositories.SpaceRecord{
				{GUID: "space1"},
			}, nil)

			securityGroupRepo.BindSecurityGroupReturns(repositories.SecurityGroupRecord{
				GUID: "test-guid",
				Name: "test-security-group",
				Rules: []repositories.SecurityGroupRule{
					{
						Protocol:    korifiv1alpha1.ProtocolTCP,
						Ports:       "80",
						Destination: "192.168.1.1",
					},
				},
				StagingSpaces: []string{"space1"},
			}, nil)
		})

		It("validates the request", func() {
			Expect(requestValidator.DecodeAndValidateJSONPayloadCallCount()).To(Equal(1))
			actualReq, _ := requestValidator.DecodeAndValidateJSONPayloadArgsForCall(0)
			Expect(bodyString(actualReq)).To(Equal("the-json-body"))
		})

		It("binds staging spaces to the security group", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(securityGroupRepo.GetSecurityGroupCallCount()).To(Equal(1))
			_, actualAuthInfo, guid := securityGroupRepo.GetSecurityGroupArgsForCall(0)

			Expect(actualAuthInfo).To(Equal(authInfo))
			Expect(guid).To(Equal("test-guid"))

			Expect(spaceRepo.ListSpacesCallCount()).To(Equal(1))
			_, _, listMessage := spaceRepo.ListSpacesArgsForCall(0)
			Expect(listMessage.GUIDs).To(ConsistOf("space1"))

			Expect(securityGroupRepo.BindSecurityGroupCallCount()).To(Equal(1))
			_, _, bindMessage := securityGroupRepo.BindSecurityGroupArgsForCall(0)
			Expect(bindMessage.GUID).To(Equal("test-guid"))
			Expect(bindMessage.Spaces).To(ConsistOf("space1"))
		})

		When("the payload is invalid", func() {
			BeforeEach(func() {
				requestValidator.DecodeAndValidateJSONPayloadReturns(errors.New("validation-error"))
			})

			It("returns a validation error", func() {
				expectUnknownError()
			})
		})

		When("the security group does not exist", func() {
			BeforeEach(func() {
				securityGroupRepo.GetSecurityGroupReturns(repositories.SecurityGroupRecord{}, apierrors.NewNotFoundError(nil, "SecurityGroup"))
			})

			It("returns a 404 not found error", func() {
				expectNotFoundError("SecurityGroup")
			})
		})

		When("the spaces does not exist", func() {
			BeforeEach(func() {
				spaceRepo.ListSpacesReturns([]repositories.SpaceRecord{}, nil)
			})

			It("returns an error", func() {
				expectUnprocessableEntityError("Space does not exist, or you do not have access.")
			})
		})

		When("fetching the spaces returns an error", func() {
			BeforeEach(func() {
				spaceRepo.ListSpacesReturns(nil, errors.New("boom!"))
			})

			It("returns an error", func() {
				expectUnknownError()
			})
		})

		When("the repository returns an error", func() {
			BeforeEach(func() {
				securityGroupRepo.BindSecurityGroupReturns(repositories.SecurityGroupRecord{}, errors.New("boom"))
			})

			It("returns an unknown error", func() {
				expectUnknownError()
			})
		})
	})
})
