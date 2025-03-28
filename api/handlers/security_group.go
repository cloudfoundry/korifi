package handlers

import (
	"context"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"
	"github.com/go-logr/logr"
)

const (
	SecurityGroupsPath = "/v3/security_groups"
)

type SecurityGroup struct {
	serverURL         url.URL
	securityGroupRepo CFSecurityGroupRepository
	spaceRepo         CFSpaceRepository
	requestValidator  RequestValidator
}

//counterfeiter:generate -o fake -fake-name CFSecurityGroupRepository . CFSecurityGroupRepository
type CFSecurityGroupRepository interface {
	CreateSecurityGroup(context.Context, authorization.Info, repositories.CreateSecurityGroupMessage) (repositories.SecurityGroupRecord, error)
}

func NewSecurityGroup(
	serverURL url.URL,
	securityGroupRepo CFSecurityGroupRepository,
	spaceRepo CFSpaceRepository,
	requestValidator RequestValidator,
) *SecurityGroup {
	return &SecurityGroup{
		serverURL:         serverURL,
		securityGroupRepo: securityGroupRepo,
		spaceRepo:         spaceRepo,
		requestValidator:  requestValidator,
	}
}

func (h *SecurityGroup) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.security-group.create")

	payload := new(payloads.SecurityGroupCreate)
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	if len(payload.Relationships.RunningSpaces.Data) != 0 || len(payload.Relationships.StagingSpaces.Data) != 0 {
		spaces, err := h.spaceRepo.ListSpaces(r.Context(), authInfo, repositories.ListSpacesMessage{
			GUIDs: append(payload.Relationships.RunningSpaces.CollectGUIDs(),
				payload.Relationships.StagingSpaces.CollectGUIDs()...),
		})
		if err != nil {
			return nil, apierrors.LogAndReturn(logger, err, "failed to list spaces for binding to security group")
		}

		if len(spaces) == 0 {
			return nil, apierrors.LogAndReturn(logger, apierrors.NewNotFoundError(nil, repositories.SpaceResourceType), "failed to bind space to security group, space not found")
		}
	}

	securityGroup, err := h.securityGroupRepo.CreateSecurityGroup(r.Context(), authInfo, payload.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Failed to create security group", "Security group Name", payload.DisplayName)
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForSecurityGroup(securityGroup, h.serverURL)), nil
}

func (h *SecurityGroup) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *SecurityGroup) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "POST", Pattern: SecurityGroupsPath, Handler: h.create},
	}
}
