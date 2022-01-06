package authorization

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=list

//counterfeiter:generate -o fake -fake-name IdentityProvider . IdentityProvider

const (
	orgLevel   = "1"
	spaceLevel = "2"
)

type IdentityProvider interface {
	GetIdentity(context.Context, Info) (Identity, error)
}

type NamespacePermissions struct {
	privilegedClient client.Client
	identityProvider IdentityProvider
	rootNamespace    string
}

func NewNamespacePermissions(privilegedClient client.Client, identityProvider IdentityProvider, rootNamespace string) *NamespacePermissions {
	return &NamespacePermissions{
		privilegedClient: privilegedClient,
		identityProvider: identityProvider,
		rootNamespace:    rootNamespace,
	}
}

func (o *NamespacePermissions) GetAuthorizedOrgNamespaces(ctx context.Context, info Info) ([]string, error) {
	return o.getAuthorizedNamespaces(ctx, info, orgLevel)
}

func (o *NamespacePermissions) GetAuthorizedSpaceNamespaces(ctx context.Context, info Info) ([]string, error) {
	return o.getAuthorizedNamespaces(ctx, info, spaceLevel)
}

func (o *NamespacePermissions) getAuthorizedNamespaces(ctx context.Context, info Info, orgSpaceLevel string) ([]string, error) {
	identity, err := o.identityProvider.GetIdentity(ctx, info)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity: %w", err)
	}

	var rolebindings rbacv1.RoleBindingList
	if err := o.privilegedClient.List(ctx, &rolebindings); err != nil {
		return nil, fmt.Errorf("failed to list rolebindings: %w", err)
	}

	var cfOrgsOrSpaces corev1.NamespaceList
	if err := o.privilegedClient.List(ctx, &cfOrgsOrSpaces, client.MatchingLabels{
		o.rootNamespace + v1alpha2.LabelTreeDepthSuffix: orgSpaceLevel,
	}); err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	cfNamespaces := map[string]bool{}
	for _, ns := range cfOrgsOrSpaces.Items {
		cfNamespaces[ns.Name] = true
	}

	var authorizedNamespaces []string
	alreadyFound := map[string]bool{}

	for _, roleBinding := range rolebindings.Items {
		for _, subject := range roleBinding.Subjects {
			if subject.Kind == identity.Kind && subject.Name == identity.Name {
				if !alreadyFound[roleBinding.Namespace] && cfNamespaces[roleBinding.Namespace] {
					authorizedNamespaces = append(authorizedNamespaces, roleBinding.Namespace)
					alreadyFound[roleBinding.Namespace] = true
				}
			}
		}
	}

	return authorizedNamespaces, nil
}

func (o *NamespacePermissions) AuthorizedIn(ctx context.Context, identity Identity, namespace string) (bool, error) {
	var rolebindings rbacv1.RoleBindingList
	err := o.privilegedClient.List(ctx, &rolebindings, client.InNamespace(namespace))
	if err != nil {
		return false, fmt.Errorf("failed to list rolebindings: %w", err)
	}

	for _, roleBinding := range rolebindings.Items {
		for _, subject := range roleBinding.Subjects {
			if subject.Kind == identity.Kind && subject.Name == identity.Name {
				return true, nil
			}
		}
	}

	return false, nil
}
