package include_test

import (
	"context"
	"errors"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers/include"
	"code.cloudfoundry.org/korifi/api/handlers/include/fake"
	"code.cloudfoundry.org/korifi/api/payloads/params"
	"code.cloudfoundry.org/korifi/api/repositories/relationships"
	"code.cloudfoundry.org/korifi/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type Foo struct {
	Name string `json:"name"`
	GUID string `json:"guid"`
}

func (t Foo) Relationships() map[string]model.ToOneRelationship {
	return map[string]model.ToOneRelationship{}
}

type Bar struct {
	Name string `json:"name"`
	GUID string `json:"guid"`
}

func (t Bar) Relationships() map[string]model.ToOneRelationship {
	return map[string]model.ToOneRelationship{}
}

var _ = Describe("ResolveIncludes", func() {
	var (
		relationshipsRepo *fake.ResourceRelationshipRepository
		resolver          *include.IncludeResolver[[]Foo, Foo]

		inputResources []Foo
		rules          []params.IncludeResourceRule
		resolveErr     error
		result         []model.IncludedResource
	)

	BeforeEach(func() {
		inputResources = []Foo{{Name: "the-foo"}}
		rules = []params.IncludeResourceRule{{
			RelationshipPath: []string{"bar"},
			Fields:           []string{},
		}}
		result = []model.IncludedResource{}

		relationshipsRepo = new(fake.ResourceRelationshipRepository)

		resolver = include.NewIncludeResolver[[]Foo](relationshipsRepo)
	})

	JustBeforeEach(func() {
		result, resolveErr = resolver.ResolveIncludes(ctx, authInfo, inputResources, rules)
	})

	It("gets the related resources from the relationship repo", func() {
		Expect(resolveErr).NotTo(HaveOccurred())
		Expect(relationshipsRepo.ListRelatedResourcesCallCount()).To(Equal(1))
		_, _, actualResType, actualResources := relationshipsRepo.ListRelatedResourcesArgsForCall(0)
		Expect(actualResType).To(Equal("bar"))
		Expect(actualResources).To(ConsistOf(Foo{Name: "the-foo"}))
	})

	It("returns no included resources", func() {
		Expect(resolveErr).NotTo(HaveOccurred())
		Expect(result).To(BeEmpty())
	})

	When("the resource has a to-one relationship", func() {
		BeforeEach(func() {
			relationshipsRepo.ListRelatedResourcesReturns([]relationships.Resource{
				Bar{
					Name: "bar",
					GUID: "bar-guid",
				},
			}, nil)
		})

		It("includes the related object", func() {
			Expect(resolveErr).NotTo(HaveOccurred())
			Expect(result).To(ConsistOf(
				model.IncludedResource{
					Type: "bars",
					Resource: map[string]any{
						"name": "bar",
						"guid": "bar-guid",
					},
				},
			))
		})

		When("particular fields are selected", func() {
			BeforeEach(func() {
				rules = []params.IncludeResourceRule{{
					RelationshipPath: []string{"bar"},
					Fields:           []string{"name"},
				}}
			})

			It("includes the selected fields of the related object", func() {
				Expect(resolveErr).NotTo(HaveOccurred())
				Expect(result).To(ConsistOf(
					model.IncludedResource{
						Type: "bars",
						Resource: map[string]any{
							"name": "bar",
						},
					},
				))
			})
		})

		When("the resource relationship is transitive", func() {
			BeforeEach(func() {
				rules = []params.IncludeResourceRule{{
					RelationshipPath: []string{"foo", "bar"},
					Fields:           []string{"name"},
				}}

				relationshipsRepo.ListRelatedResourcesReturns([]relationships.Resource{
					Bar{
						Name: "bar",
					},
				}, nil)
			})

			It("gets the related resources for each resource type in the relationship", func() {
				Expect(resolveErr).NotTo(HaveOccurred())
				Expect(relationshipsRepo.ListRelatedResourcesCallCount()).To(Equal(2))

				_, _, actualResType, actualResources := relationshipsRepo.ListRelatedResourcesArgsForCall(0)
				Expect(actualResType).To(Equal("foo"))
				Expect(actualResources).To(ConsistOf(Foo{Name: "the-foo"}))

				_, _, actualResType, actualResources = relationshipsRepo.ListRelatedResourcesArgsForCall(1)
				Expect(actualResType).To(Equal("bar"))
				Expect(actualResources).To(ConsistOf(Bar{Name: "bar"}))
			})
		})

		When("there are multiple rules", func() {
			BeforeEach(func() {
				rules = []params.IncludeResourceRule{
					{
						RelationshipPath: []string{"foo"},
					},
					{
						RelationshipPath: []string{"bar"},
						Fields:           []string{"name"},
					},
				}

				relationshipsRepo.ListRelatedResourcesStub = func(
					_ context.Context, _ authorization.Info, resType string, _ []relationships.Resource,
				) ([]relationships.Resource, error) {
					switch resType {
					case "foo":
						return []relationships.Resource{
							Foo{
								Name: "foo",
								GUID: "foo-guid",
							},
						}, nil
					case "bar":
						return []relationships.Resource{
							Bar{
								Name: "bar",
								GUID: "bar-guid",
							},
						}, nil
					}
					return nil, nil
				}
			})

			It("resolves all rules", func() {
				Expect(resolveErr).NotTo(HaveOccurred())
				Expect(result).To(ConsistOf(
					model.IncludedResource{
						Type: "foos",
						Resource: map[string]any{
							"name": "foo",
							"guid": "foo-guid",
						},
					},
					model.IncludedResource{
						Type: "bars",
						Resource: map[string]any{
							"name": "bar",
						},
					},
				))
			})
		})

		When("the relationship repository errors", func() {
			BeforeEach(func() {
				relationshipsRepo.ListRelatedResourcesReturns(nil, errors.New("relationship-repo-error"))
			})

			It("returns an error", func() {
				Expect(resolveErr).To(MatchError("relationship-repo-error"))
			})
		})
	})
})
