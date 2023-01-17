package payloads_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("PackageCreate", func() {
	var (
		createPayload payloads.PackageCreate
		packageCreate *payloads.PackageCreate
		validatorErr  error
	)

	BeforeEach(func() {
		packageCreate = new(payloads.PackageCreate)
		createPayload = payloads.PackageCreate{
			Type: "bits",
			Relationships: &payloads.PackageRelationships{
				App: &payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: "some-guid",
					},
				},
			},
			Metadata: payloads.Metadata{
				Labels: map[string]string{
					"foo": "bar",
				},
				Annotations: map[string]string{
					"example.org/jim": "hello",
				},
			},
		}
	})

	JustBeforeEach(func() {
		body, err := json.Marshal(createPayload)
		Expect(err).NotTo(HaveOccurred())

		req, err := http.NewRequest("", "", bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())

		validatorErr = validator.DecodeAndValidateJSONPayload(req, packageCreate)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(packageCreate).To(gstruct.PointTo(Equal(createPayload)))
	})

	When("type is empty", func() {
		BeforeEach(func() {
			createPayload.Type = ""
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Type is a required field")
		})
	})

	When("type is not in the allowed list", func() {
		BeforeEach(func() {
			createPayload.Type = "foo"
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Type must be one of ['bits']")
		})
	})

	When("relationships is not set", func() {
		BeforeEach(func() {
			createPayload.Relationships = nil
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Relationships is a required field")
		})
	})

	When("relationships.app is not set", func() {
		BeforeEach(func() {
			createPayload.Relationships.App = nil
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "App is a required field")
		})
	})

	When("relationships.app.data is not set", func() {
		BeforeEach(func() {
			createPayload.Relationships.App.Data = nil
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "Data is a required field")
		})
	})

	When("relationships.app.data.guid is not set", func() {
		BeforeEach(func() {
			createPayload.Relationships.App.Data.GUID = ""
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "GUID is a required field")
		})
	})

	When("metadata.labels contains an invalid key", func() {
		BeforeEach(func() {
			createPayload.Metadata = payloads.Metadata{
				Labels: map[string]string{
					"foo.cloudfoundry.org/bar": "jim",
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
		})
	})

	When("metadata.annotations contains an invalid key", func() {
		BeforeEach(func() {
			createPayload.Metadata = payloads.Metadata{
				Annotations: map[string]string{
					"foo.cloudfoundry.org/bar": "jim",
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
		})
	})
})

var _ = Describe("PackageUpdate", func() {
	var (
		updatePayload payloads.PackageUpdate
		packageUpdate *payloads.PackageUpdate
		validatorErr  error
	)

	BeforeEach(func() {
		packageUpdate = new(payloads.PackageUpdate)
		updatePayload = payloads.PackageUpdate{
			Metadata: payloads.MetadataPatch{
				Labels: map[string]*string{
					"foo": tools.PtrTo("bar"),
					"bar": nil,
				},
				Annotations: map[string]*string{
					"example.org/jim": tools.PtrTo("hello"),
				},
			},
		}
	})

	JustBeforeEach(func() {
		body, err := json.Marshal(updatePayload)
		Expect(err).NotTo(HaveOccurred())

		req, err := http.NewRequest("", "", bytes.NewReader(body))
		Expect(err).NotTo(HaveOccurred())

		validatorErr = validator.DecodeAndValidateJSONPayload(req, packageUpdate)
	})

	It("succeeds", func() {
		Expect(validatorErr).NotTo(HaveOccurred())
		Expect(packageUpdate).To(gstruct.PointTo(Equal(updatePayload)))
	})

	When("metadata.labels contains an invalid key", func() {
		BeforeEach(func() {
			updatePayload.Metadata = payloads.MetadataPatch{
				Labels: map[string]*string{
					"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
		})
	})

	When("metadata.annotations contains an invalid key", func() {
		BeforeEach(func() {
			updatePayload.Metadata = payloads.MetadataPatch{
				Annotations: map[string]*string{
					"foo.cloudfoundry.org/bar": tools.PtrTo("jim"),
				},
			}
		})

		It("returns an appropriate error", func() {
			expectUnprocessableEntityError(validatorErr, "cannot begin with \"cloudfoundry.org\"")
		})
	})

	Context("toMessage", func() {
		It("converts to repo message correctly", func() {
			msg := packageUpdate.ToMessage("foo")
			Expect(msg.MetadataPatch.Labels).To(Equal(map[string]*string{
				"foo": tools.PtrTo("bar"),
				"bar": nil,
			}))
		})
	})
})

var _ = Describe("PackageListQueryParameters", func() {
	Describe("DecodeFromURLValues", func() {
		packageList := payloads.PackageListQueryParameters{}
		err := packageList.DecodeFromURLValues(url.Values{
			"app_guids": []string{"app_guid"},
			"states":    []string{"state"},
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(packageList).To(Equal(payloads.PackageListQueryParameters{
			AppGUIDs: "app_guid",
			States:   "state",
		}))
	})
})
