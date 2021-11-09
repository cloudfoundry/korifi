package authorization

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=list

type Org struct {
	k8sClient client.Client
}

func NewOrg(k8sClient client.Client) *Org {
	return &Org{
		k8sClient: k8sClient,
	}
}

func (o *Org) GetAuthorizedNamespaces(ctx context.Context, identity Identity) ([]string, error) {
	var authorizedNamespaces []string

	var rolebindings rbacv1.RoleBindingList
	err := o.k8sClient.List(ctx, &rolebindings)
	if err != nil {
		return nil, fmt.Errorf("failed to list rolebindings: %w", err)
	}

	nsMap := map[string]bool{}
	for _, roleBinding := range rolebindings.Items {
		for _, subject := range roleBinding.Subjects {
			if subject.Kind == identity.Kind && subject.Name == identity.Name {
				if !nsMap[roleBinding.Namespace] {
					authorizedNamespaces = append(authorizedNamespaces, roleBinding.Namespace)
					nsMap[roleBinding.Namespace] = true
				}
			}
		}
	}

	return authorizedNamespaces, nil
}

func (o *Org) AuthorizedIn(ctx context.Context, identity Identity, namespace string) (bool, error) {
	var rolebindings rbacv1.RoleBindingList
	err := o.k8sClient.List(ctx, &rolebindings, client.InNamespace(namespace))
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
