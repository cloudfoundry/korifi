package model_test

import (
	"code.cloudfoundry.org/korifi/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("IncludedResource", func() {
	var resource model.IncludedResource

	BeforeEach(func() {
		resource = model.IncludedResource{
			Type: "my-resource-type",
			Resource: struct {
				StringField string `json:"string_field"`
				IntField    int    `json:"int_field"`
				StructField struct {
					Foo string `json:"foo"`
				} `json:"struct_field"`
			}{
				StringField: "my_string",
				IntField:    5,
				StructField: struct {
					Foo string `json:"foo"`
				}{
					Foo: "bar",
				},
			},
		}
	})

	Describe("SelectJSONFields", func() {
		var (
			resourceWithFields model.IncludedResource
			fields             []string
		)

		BeforeEach(func() {
			fields = []string{}
		})

		JustBeforeEach(func() {
			var err error
			resourceWithFields, err = resource.SelectJSONFields(fields...)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns a resource with all fields", func() {
			Expect(resourceWithFields).To(MatchAllFields(Fields{
				"Type": Equal("my-resource-type"),
				"Resource": MatchAllKeys(Keys{
					"string_field": Equal("my_string"),
					"int_field":    BeEquivalentTo(5),
					"struct_field": MatchAllKeys(Keys{
						"foo": Equal("bar"),
					}),
				}),
			}))
		})

		When("fields are selected", func() {
			BeforeEach(func() {
				fields = []string{"int_field"}
			})

			It("returns a resource with selected fields only", func() {
				Expect(resourceWithFields).To(MatchAllFields(Fields{
					"Type": Equal("my-resource-type"),
					"Resource": MatchAllKeys(Keys{
						"int_field": BeEquivalentTo(5),
					}),
				}))
			})
		})
	})
})
