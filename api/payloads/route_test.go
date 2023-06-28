package payloads_test

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("RouteList", func() {
	Describe("decode from url values", func() {
		var (
			routeList payloads.RouteList
			decodeErr error
			params    string
		)

		BeforeEach(func() {
			routeList = payloads.RouteList{}
			params = "app_guids=app_guid&space_guids=space_guid&domain_guids=domain_guid&hosts=host&paths=path"
		})

		JustBeforeEach(func() {
			req, err := http.NewRequest("GET", "http://foo.com/bar?"+params, nil)
			Expect(err).NotTo(HaveOccurred())
			decodeErr = validator.DecodeAndValidateURLValues(req, &routeList)
		})

		It("succeeds", func() {
			Expect(decodeErr).NotTo(HaveOccurred())
			Expect(routeList).To(Equal(payloads.RouteList{
				AppGUIDs:    "app_guid",
				SpaceGUIDs:  "space_guid",
				DomainGUIDs: "domain_guid",
				Hosts:       "host",
				Paths:       "path",
			}))
		})

		When("it contains an invalid key", func() {
			BeforeEach(func() {
				params = "foo=bar"
			})

			It("fails", func() {
				Expect(decodeErr).To(MatchError("unsupported query parameter: foo"))
			})
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
					Port:     tools.PtrTo(1234),
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
