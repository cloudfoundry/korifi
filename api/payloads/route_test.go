package payloads_test

import (
	"code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("RouteList", func() {
	Describe("Validation", func() {
		DescribeTable("valid query",
			func(query string, expectedRouteList payloads.RouteList) {
				actualRouteList, decodeErr := decodeQuery[payloads.RouteList](query)

				Expect(decodeErr).NotTo(HaveOccurred())
				Expect(*actualRouteList).To(Equal(expectedRouteList))
			},

			Entry("app_guids", "app_guids=guid1,guid2", payloads.RouteList{AppGUIDs: "guid1,guid2"}),
			Entry("space_guids", "space_guids=guid1,guid2", payloads.RouteList{SpaceGUIDs: "guid1,guid2"}),
			Entry("domain_guids", "domain_guids=guid1,guid2", payloads.RouteList{DomainGUIDs: "guid1,guid2"}),
			Entry("hosts", "hosts=h1,h2", payloads.RouteList{Hosts: "h1,h2"}),
			Entry("paths", "paths=h1,h2", payloads.RouteList{Paths: "h1,h2"}),
			Entry("order_by created_at", "order_by=created_at", payloads.RouteList{OrderBy: "created_at"}),
			Entry("order_by -created_at", "order_by=-created_at", payloads.RouteList{OrderBy: "-created_at"}),
			Entry("order_by updated_at", "order_by=updated_at", payloads.RouteList{OrderBy: "updated_at"}),
			Entry("order_by -updated_at", "order_by=-updated_at", payloads.RouteList{OrderBy: "-updated_at"}),
			Entry("page=3", "page=3", payloads.RouteList{Pagination: payloads.Pagination{Page: "3"}}),
		)

		DescribeTable("invalid query",
			func(query string, expectedErrMsg string) {
				_, decodeErr := decodeQuery[payloads.RouteList](query)
				Expect(decodeErr).To(MatchError(ContainSubstring(expectedErrMsg)))
			},
			Entry("invalid order_by", "order_by=foo", "one of"),
			Entry("per_page is not a number", "per_page=foo", "value must be an integer"),
		)
	})

	Describe("ToMessage", func() {
		var (
			processList payloads.RouteList
			message     repositories.ListRoutesMessage
		)

		BeforeEach(func() {
			processList = payloads.RouteList{
				AppGUIDs:    "ag1,ag2",
				SpaceGUIDs:  "sg1,sg2",
				DomainGUIDs: "dg1,dg2",
				Hosts:       "h1,h2",
				Paths:       "p1,p2",
				OrderBy:     "created_at",
				Pagination: payloads.Pagination{
					PerPage: "10",
					Page:    "4",
				},
			}
		})

		JustBeforeEach(func() {
			message = processList.ToMessage()
		})

		It("translates to repository message", func() {
			Expect(message).To(Equal(repositories.ListRoutesMessage{
				AppGUIDs:    []string{"ag1", "ag2"},
				SpaceGUIDs:  []string{"sg1", "sg2"},
				DomainGUIDs: []string{"dg1", "dg2"},
				Hosts:       []string{"h1", "h2"},
				Paths:       []string{"p1", "p2"},
				OrderBy:     "created_at",
				Pagination: repositories.Pagination{
					PerPage: 10,
					Page:    4,
				},
			}))
		})
	})
})

var _ = Describe("RouteCreate", func() {
	var (
		createPayload payloads.RouteCreate
		routeCreate   *payloads.RouteCreate
		validatorErr  error
		apiError      errors.ApiError
	)

	BeforeEach(func() {
		routeCreate = new(payloads.RouteCreate)
		createPayload = payloads.RouteCreate{
			Host: "h1",
			Path: "p1",
			Port: 8080,
			Relationships: &payloads.RouteRelationships{
				Domain: payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: "d1",
					},
				},
				Space: payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: "s1",
					},
				},
			},
			Metadata: payloads.Metadata{
				Annotations: map[string]string{"a": "av"},
				Labels:      map[string]string{"l": "lv"},
			},
		}
	})

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(createPayload), routeCreate)
		apiError, _ = validatorErr.(errors.ApiError)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(routeCreate).To(gstruct.PointTo(Equal(createPayload)))
	})

	When("host is blank", func() {
		BeforeEach(func() {
			createPayload.Host = ""
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("host cannot be blank"))
		})
	})

	When("relationships is empty", func() {
		BeforeEach(func() {
			createPayload.Relationships = nil
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("relationships is required"))
		})
	})

	When("domain guid is blank", func() {
		BeforeEach(func() {
			createPayload.Relationships.Domain.Data.GUID = ""
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("relationships.domain.data.guid cannot be blank"))
		})
	})

	When("space guid is blank", func() {
		BeforeEach(func() {
			createPayload.Relationships.Space.Data.GUID = ""
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("relationships.space.data.guid cannot be blank"))
		})
	})

	When("metadata uses the cloudfoundry domain", func() {
		BeforeEach(func() {
			createPayload.Metadata.Labels["foo.cloudfoundry.org/bar"] = "baz"
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("cannot use the cloudfoundry.org domain"))
		})
	})
})

var _ = Describe("RoutePatch", func() {
	var (
		patchPayload payloads.RoutePatch
		routePatch   *payloads.RoutePatch
		validatorErr error
		apiError     errors.ApiError
	)

	BeforeEach(func() {
		routePatch = new(payloads.RoutePatch)
		patchPayload = payloads.RoutePatch{
			Metadata: payloads.MetadataPatch{
				Annotations: map[string]*string{"a": tools.PtrTo("av")},
				Labels:      map[string]*string{"l": tools.PtrTo("lv")},
			},
		}
	})

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(patchPayload), routePatch)
		apiError, _ = validatorErr.(errors.ApiError)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(routePatch).To(gstruct.PointTo(Equal(patchPayload)))
	})

	When("metadata uses the cloudfoundry domain", func() {
		BeforeEach(func() {
			patchPayload.Metadata.Labels["foo.cloudfoundry.org/bar"] = tools.PtrTo("baz")
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("cannot use the cloudfoundry.org domain"))
		})
	})
})

var _ = Describe("Add destination", func() {
	var (
		addPayload     payloads.RouteDestinationCreate
		destinationAdd *payloads.RouteDestinationCreate
		validatorErr   error
		apiError       errors.ApiError
	)

	BeforeEach(func() {
		destinationAdd = new(payloads.RouteDestinationCreate)
		addPayload = payloads.RouteDestinationCreate{
			Destinations: []payloads.RouteDestination{
				{
					App: payloads.AppResource{
						GUID: "app-1-guid",
					},
				},
				{
					App: payloads.AppResource{
						GUID: "app-2-guid",
						Process: &payloads.DestinationAppProcess{
							Type: "queue",
						},
					},
					Port:     tools.PtrTo[int32](1234),
					Protocol: tools.PtrTo("http1"),
				},
			},
		}
	})

	JustBeforeEach(func() {
		validatorErr = validator.DecodeAndValidateJSONPayload(createJSONRequest(addPayload), destinationAdd)
		apiError, _ = validatorErr.(errors.ApiError)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(destinationAdd).To(gstruct.PointTo(Equal(addPayload)))
	})

	When("app guid is empty", func() {
		BeforeEach(func() {
			addPayload.Destinations[0].App.GUID = ""
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("guid cannot be blank"))
		})
	})

	When("process type is empty", func() {
		BeforeEach(func() {
			addPayload.Destinations[1].App.Process.Type = ""
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("type cannot be blank"))
		})
	})

	When("protocol is not http1", func() {
		BeforeEach(func() {
			addPayload.Destinations[1].Protocol = tools.PtrTo("http")
		})

		It("fails", func() {
			Expect(apiError).To(HaveOccurred())
			Expect(apiError.Detail()).To(ContainSubstring("value must be one of: http1"))
		})
	})
})
