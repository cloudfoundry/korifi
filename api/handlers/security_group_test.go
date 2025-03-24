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

	Describe("POST /v3/security_groups", func() {
		var payload payloads.SecurityGroupCreate

		BeforeEach(func() {
			requestMethod = http.MethodPost
			requestPath = "/v3/security_groups"
			requestBody = "the-json-body"

			payload = payloads.SecurityGroupCreate{
				DisplayName: "test-security-group",
				Rules: []korifiv1alpha1.SecurityGroupRule{
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
				Rules: []korifiv1alpha1.SecurityGroupRule{
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
			Expect(createSecurityGroupMessage.Rules).To(Equal([]korifiv1alpha1.SecurityGroupRule{
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
				spaceRepo.ListSpacesReturns([]repositories.SpaceRecord{}, apierrors.NewNotFoundError(nil, repositories.SecurityGroupResourceType))
			})

			It("returns a not found error", func() {
				expectNotFoundError(repositories.SecurityGroupResourceType)
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
})
