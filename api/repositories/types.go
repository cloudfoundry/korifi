package repositories

import (
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"context"
)

//counterfeiter:generate -o fake -fake-name NamespacePermissions . NamespacePermissions
type NamespacePermissions interface {
	GetAuthorizedOrgNamespaces(ctx context.Context, info authorization.Info) (map[string]bool, error)
	GetAuthorizedSpaceNamespaces(ctx context.Context, info authorization.Info) (map[string]bool, error)
	AuthorizedIn(ctx context.Context, identity authorization.Identity, namespace string) (bool, error)
}
