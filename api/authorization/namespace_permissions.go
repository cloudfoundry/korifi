package authorization

import (
	"context"
	"fmt"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=list
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=list

//counterfeiter:generate -o fake -fake-name IdentityProvider . IdentityProvider

type IdentityProvider interface {
	GetIdentity(context.Context, Info) (Identity, error)
}

type NamespacePermissions struct {
	privilegedClient client.Client
	identityProvider IdentityProvider
}

func NewNamespacePermissions(privilegedClient client.Client, identityProvider IdentityProvider) *NamespacePermissions {
	return &NamespacePermissions{
		privilegedClient: privilegedClient,
		identityProvider: identityProvider,
	}
}

func (o *NamespacePermissions) GetAuthorizedOrgNamespaces(ctx context.Context, info Info) (map[string]bool, error) {
	return o.getAuthorizedNamespaces(ctx, info, korifiv1alpha1.OrgNameKey, "Org")
}

func (o *NamespacePermissions) GetAuthorizedSpaceNamespaces(ctx context.Context, info Info) (map[string]bool, error) {
	return o.getAuthorizedNamespaces(ctx, info, korifiv1alpha1.SpaceNameKey, "Space")
}

func (o *NamespacePermissions) getAuthorizedNamespaces(ctx context.Context, info Info, orgSpaceLabel, resourceType string) (map[string]bool, error) {
	identity, err := o.identityProvider.GetIdentity(ctx, info)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity: %w", err)
	}

	var rolebindings rbacv1.RoleBindingList
	if err := o.privilegedClient.List(ctx, &rolebindings); err != nil {
		return nil, fmt.Errorf("failed to list rolebindings: %w", apierrors.FromK8sError(err, resourceType))
	}

	var cfOrgsOrSpaces corev1.NamespaceList
	if err := o.privilegedClient.List(ctx, &cfOrgsOrSpaces, client.HasLabels([]string{orgSpaceLabel})); err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", apierrors.FromK8sError(err, resourceType))
	}

	cfNamespaces := map[string]bool{}
	for _, ns := range cfOrgsOrSpaces.Items {
		cfNamespaces[ns.Name] = true
	}

	authorizedNamespaces := map[string]bool{}

	for _, roleBinding := range rolebindings.Items {
		for _, subject := range roleBinding.Subjects {
			isMatch, err := SameSubject(subject, identity)
			if err != nil {
				return nil, err
			}
			if isMatch {
				if cfNamespaces[roleBinding.Namespace] {
					authorizedNamespaces[roleBinding.Namespace] = true
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
		return false, fmt.Errorf("failed to list rolebindings: %w", apierrors.FromK8sError(err, ""))
	}

	for _, roleBinding := range rolebindings.Items {
		for _, subject := range roleBinding.Subjects {
			isMatch, err := SameSubject(subject, identity)
			if err != nil {
				return false, err
			}
			if isMatch {
				return true, nil
			}
		}
	}

	return false, nil
}

func SameSubject(subject rbacv1.Subject, identity Identity) (bool, error) {
	if identity.Kind != subject.Kind {
		return false, nil
	}

	if identity.Kind == "ServiceAccount" {
		if !HasServiceAccountPrefix(identity.Name) {
			return false, fmt.Errorf("expected user identifier %q to have prefix %q", identity.Name, serviceAccountNamePrefix)
		}
		_, identitySAName := ServiceAccountNSAndName(identity.Name)
		return identitySAName == subject.Name, nil
	} else {
		return identity.Name == subject.Name, nil
	}
}

func ServiceAccountNSAndName(serviceAccountSubjectName string) (string, string) {
	nameSegments := strings.Split(serviceAccountSubjectName, ":")

	serviceAccountNS := nameSegments[len(nameSegments)-2]
	serviceAccountName := nameSegments[len(nameSegments)-1]
	return serviceAccountNS, serviceAccountName
}
