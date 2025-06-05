package values_test

import (
	"errors"

	"code.cloudfoundry.org/korifi/controllers/webhooks/label_indexer/values"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var _ = Describe("IndexValues", func() {
	var (
		result *string
		err    error
	)

	Describe("JSONValue", func() {
		var (
			obj  map[string]any
			path string
		)

		BeforeEach(func() {
			obj = map[string]any{
				"foo": map[string]any{
					"bar": "baz",
				},
			}

			path = "$.foo.bar"
		})

		JustBeforeEach(func() {
			result, err = values.JSONValue(path)(obj)
		})

		It("returns the value at the given JSON path", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(PointTo(Equal(`"baz"`)))
		})

		When("the path does not exist", func() {
			BeforeEach(func() {
				path = "$.foo.qux"
			})

			It("returns nil", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})

		When("the object contains an array", func() {
			BeforeEach(func() {
				obj = map[string]any{
					"students": []any{
						map[string]any{
							"first_name": "Alice",
							"last_name":  "Smith",
						},
						map[string]any{
							"first_name": "Bob",
							"last_name":  "Johnson",
						},
					},
				}
				path = `$.students[?@.last_name == "Smith"].first_name`
			})

			It("returns a filtered array of results", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(PointTo(Equal(`["Alice"]`)))
			})
		})
	})

	Describe("SingleValue", func() {
		var indexingFunc values.IndexValueFunc

		BeforeEach(func() {
			indexingFunc = func(obj map[string]any) (*string, error) {
				return tools.PtrTo(`["foo"]`), nil
			}
		})

		JustBeforeEach(func() {
			result, err = values.SingleValue(indexingFunc)(map[string]any{})
		})

		It("returns the first value in the array", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(PointTo(Equal(`"foo"`)))
		})

		When("indexing func returns empty array", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return tools.PtrTo(`[]`), nil
				}
			})

			It("returns nil", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})

		When("indexing func reuturns array with more than one element", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return tools.PtrTo(`["foo", "bar"]`), nil
				}
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("expected single value, got array")))
			})
		})

		When("indexing func returns invalid json", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return tools.PtrTo(`{"foo": "bar"`), nil
				}
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("failed to unmarshal value")))
			})
		})

		When("indexing func returns an error", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return nil, errors.New("foo")
				}
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("foo")))
			})
		})

		When("indexing func returns nil", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return nil, nil
				}
			})

			It("returns nil", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})
	})

	Describe("IsEmptyValue", func() {
		var indexingFunc values.IndexValueFunc

		BeforeEach(func() {
			indexingFunc = func(obj map[string]any) (*string, error) {
				return tools.PtrTo(`["foo"]`), nil
			}
		})

		JustBeforeEach(func() {
			result, err = values.IsEmptyValue(indexingFunc)(map[string]any{})
		})

		It("returns false", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(PointTo(Equal("false")))
		})

		When("indexing func returns empty array", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return tools.PtrTo(`[]`), nil
				}
			})

			It("returns true", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(PointTo(Equal("true")))
			})
		})

		When("indexing func returns invalid json", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return tools.PtrTo(`{"foo": "bar"`), nil
				}
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("failed to unmarshal value")))
			})
		})

		When("indexing func returns an error", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return nil, errors.New("foo")
				}
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("foo")))
			})
		})

		When("indexing func returns nil", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return nil, nil
				}
			})

			It("returns nil", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})
	})

	Describe("Unquote", func() {
		var indexingFunc values.IndexValueFunc

		BeforeEach(func() {
			indexingFunc = func(obj map[string]any) (*string, error) {
				return tools.PtrTo(`"foo"`), nil
			}
		})

		JustBeforeEach(func() {
			result, err = values.Unquote(indexingFunc)(map[string]any{})
		})

		It("returns the unquoted value", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(PointTo(Equal("foo")))
		})

		When("indexing func returns an error", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return nil, errors.New("foo")
				}
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("foo")))
			})
		})

		When("indexing func returns nil", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return nil, nil
				}
			})

			It("returns nil", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})

		When("indexing func returns an unquoted value", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return tools.PtrTo("foo"), nil
				}
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("failed to unquote value")))
			})
		})
	})

	Describe("SHA224", func() {
		var indexingFunc values.IndexValueFunc

		BeforeEach(func() {
			indexingFunc = func(obj map[string]any) (*string, error) {
				return tools.PtrTo("foo"), nil
			}
		})

		JustBeforeEach(func() {
			result, err = values.SHA224(indexingFunc)(map[string]any{})
		})

		It("returns the SHA224 hash of the value", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(PointTo(Equal("0808f64e60d58979fcb676c96ec938270dea42445aeefcd3a4e6f8db")))
		})

		When("indexing func returns an error", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return nil, errors.New("foo")
				}
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("foo")))
			})
		})

		When("indexing func returns nil", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return nil, nil
				}
			})

			It("returns nil", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})
	})

	Describe("EmptyValue", func() {
		JustBeforeEach(func() {
			result, err = values.EmptyValue()(map[string]any{})
		})

		It("returns an empty string", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(PointTo(Equal("")))
		})
	})

	Describe("ConstantValue", func() {
		JustBeforeEach(func() {
			result, err = values.ConstantValue("foo")(map[string]any{})
		})

		It("returns the value", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(PointTo(Equal("foo")))
		})
	})

	Describe("Map", func() {
		var (
			indexingFunc values.IndexValueFunc
			mapping      map[string]values.IndexValueFunc
		)

		BeforeEach(func() {
			indexingFunc = func(obj map[string]any) (*string, error) {
				return tools.PtrTo("foo"), nil
			}

			mapping = map[string]values.IndexValueFunc{
				"foo": func(map[string]any) (*string, error) {
					return tools.PtrTo("bar"), nil
				},
			}
		})

		JustBeforeEach(func() {
			result, err = values.Map(indexingFunc, mapping)(map[string]any{})
		})

		It("returns the mapped value", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(PointTo(Equal("bar")))
		})

		When("the previous func returns an error", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return nil, errors.New("foo")
				}
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("foo")))
			})
		})

		When("the mapped func returns an error", func() {
			BeforeEach(func() {
				mapping = map[string]values.IndexValueFunc{
					"foo": func(map[string]any) (*string, error) {
						return nil, errors.New("bar")
					},
				}
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("bar")))
			})
		})

		When("no mapping is found for the previous value", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return tools.PtrTo("foo1"), nil
				}
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("no mapping found")))
			})
		})

		When("the previous func returns nil", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return nil, nil
				}
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("cannot map nil")))
			})
		})
	})

	Describe("DefaultIfEmpty", func() {
		var (
			indexingFunc   values.IndexValueFunc
			defaultingFunc values.IndexValueFunc
		)

		BeforeEach(func() {
			indexingFunc = func(obj map[string]any) (*string, error) {
				return tools.PtrTo("foo"), nil
			}

			defaultingFunc = func(map[string]any) (*string, error) {
				return tools.PtrTo("bar"), nil
			}
		})

		JustBeforeEach(func() {
			result, err = values.DefaultIfEmpty(indexingFunc, defaultingFunc)(map[string]any{})
		})

		It("returns the previous value", func() {
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(PointTo(Equal("foo")))
		})

		When("the previous value is nil", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return nil, nil
				}
			})

			It("returns the default value", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(PointTo(Equal("bar")))
			})

			When("the defaulting func returns an error", func() {
				BeforeEach(func() {
					defaultingFunc = func(obj map[string]any) (*string, error) {
						return nil, errors.New("bar")
					}
				})

				It("returns an error", func() {
					Expect(err).To(MatchError(ContainSubstring("bar")))
				})
			})
		})

		When("the previous func returns an error", func() {
			BeforeEach(func() {
				indexingFunc = func(obj map[string]any) (*string, error) {
					return nil, errors.New("foo")
				}
			})

			It("returns an error", func() {
				Expect(err).To(MatchError(ContainSubstring("foo")))
			})
		})
	})
})
