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

func (t Foo) Relationships() map[string]string {
	return nil
}

type Bar struct {
	Name string `json:"name"`
	GUID string `json:"guid"`
}

func (t Bar) Relationships() map[string]string {
	return nil
}

var _ = Describe("ResolveIncludes", func() {
	var (
		relationshipsRepo *fake.ResourceRelationshipRepository
		resourcePresenter *fake.ResourcePresenter
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
		resourcePresenter = new(fake.ResourcePresenter)
		resourcePresenter.PresentResourceReturns(map[string]string{
			"presented_field1": "present1",
			"presented_field2": "present2",
		})

		resolver = include.NewIncludeResolver[[]Foo](relationshipsRepo, resourcePresenter)
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

		It("presents the related object", func() {
			Expect(resourcePresenter.PresentResourceCallCount()).To(Equal(1))
			actualResource := resourcePresenter.PresentResourceArgsForCall(0)
			Expect(actualResource).To(Equal(Bar{
				Name: "bar",
				GUID: "bar-guid",
			}))
		})

		It("includes the presented object", func() {
			Expect(resolveErr).NotTo(HaveOccurred())
			Expect(result).To(ConsistOf(
				model.IncludedResource{
					Type: "bars",
					Resource: map[string]any{
						"presented_field1": "present1",
						"presented_field2": "present2",
					},
				},
			))
		})

		When("particular fields are selected", func() {
			BeforeEach(func() {
				rules = []params.IncludeResourceRule{{
					RelationshipPath: []string{"bar"},
					Fields:           []string{"presented_field2"},
				}}
			})

			It("includes the selected fields of the related object", func() {
				Expect(resolveErr).NotTo(HaveOccurred())
				Expect(result).To(ConsistOf(
					model.IncludedResource{
						Type: "bars",
						Resource: map[string]any{
							"presented_field2": "present2",
						},
					},
				))
			})
		})

		When("the resource relationship is transitive", func() {
			BeforeEach(func() {
				rules = []params.IncludeResourceRule{{
					RelationshipPath: []string{"foo", "bar"},
					Fields:           []string{"presented_field1"},
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

				resourcePresenter.PresentResourceReturnsOnCall(0, map[string]string{
					"guid": "presented-foo-guid",
					"name": "presented-foo-name",
				})
				resourcePresenter.PresentResourceReturnsOnCall(1, map[string]string{
					"guid": "presented-bar-guid",
					"name": "presented-bar-name",
				})
			})

			It("resolves all rules", func() {
				Expect(resolveErr).NotTo(HaveOccurred())
				Expect(result).To(ConsistOf(
					model.IncludedResource{
						Type: "foos",
						Resource: map[string]any{
							"guid": "presented-foo-guid",
							"name": "presented-foo-name",
						},
					},
					model.IncludedResource{
						Type: "bars",
						Resource: map[string]any{
							"name": "presented-bar-name",
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
