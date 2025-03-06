package include

import (
	"context"
	"slices"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads/params"
	"code.cloudfoundry.org/korifi/api/repositories/relationships"
	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
)

//counterfeiter:generate -o fake -fake-name ResourceRelationshipRepository . ResourceRelationshipRepository
type ResourceRelationshipRepository interface {
	ListRelatedResources(context.Context, authorization.Info, string, []relationships.Resource) ([]relationships.Resource, error)
}

//counterfeiter:generate -o fake -fake-name ResourcePresenter . ResourcePresenter
type ResourcePresenter interface {
	PresentResource(resource relationships.Resource) any
}

type IncludeResolver[S ~[]E, E relationships.Resource] struct {
	relationshipsRepo ResourceRelationshipRepository
	resourcePresenter ResourcePresenter
}

func NewIncludeResolver[S ~[]E, E relationships.Resource](
	relationshipsRepo ResourceRelationshipRepository,
	resourcePresenter ResourcePresenter,
) *IncludeResolver[S, E] {
	return &IncludeResolver[S, E]{
		relationshipsRepo: relationshipsRepo,
		resourcePresenter: resourcePresenter,
	}
}

func (h *IncludeResolver[S, E]) ResolveIncludes(
	ctx context.Context,
	authInfo authorization.Info,
	resources S,
	includeResourceRules []params.IncludeResourceRule,
) ([]Resource, error) {
	includes := []Resource{}

	repoResources := slices.Collect(it.Map(itx.FromSlice(resources), func(e E) relationships.Resource {
		return e
	}))

	for _, includeResourceRule := range includeResourceRules {
		includedResources, err := h.resolveInclude(ctx, authInfo, repoResources, includeResourceRule.RelationshipPath)
		if err != nil {
			return nil, err
		}

		partialResources, err := selectFields(includedResources, includeResourceRule.Fields)
		if err != nil {
			return nil, err
		}

		includes = append(includes, partialResources...)
	}

	return includes, nil
}

func (h *IncludeResolver[S, E]) resolveInclude(
	ctx context.Context,
	authInfo authorization.Info,
	resources []relationships.Resource,
	relationshipPath []string,
) ([]Resource, error) {
	var includedResources []Resource

	for _, relatedResourceType := range relationshipPath {
		var err error
		resources, err = h.relationshipsRepo.ListRelatedResources(ctx, authInfo, relatedResourceType, resources)
		if err != nil {
			return nil, err
		}

		includedResources = slices.Collect(it.Map(itx.FromSlice(resources), func(r relationships.Resource) Resource {
			return Resource{
				Type:     plural(relatedResourceType),
				Resource: h.resourcePresenter.PresentResource(r),
			}
		}))
	}

	return includedResources, nil
}

func selectFields(includedResources []Resource, fields []string) ([]Resource, error) {
	res := []Resource{}

	for _, includedResource := range includedResources {
		partialResource, err := includedResource.SelectJSONPaths(fields...)
		if err != nil {
			return nil, err
		}

		res = append(res, partialResource)
	}

	return res, nil
}

func plural(s string) string {
	return s + "s"
}
