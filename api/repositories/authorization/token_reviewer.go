package authorization

import (
	"context"
	"fmt"
	"strings"

	authv1 "k8s.io/api/authentication/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create

const (
	serviceAccountsGroup     = "system:serviceaccounts"
	serviceAccountNamePrefix = "system:serviceaccount:"
)

//+kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create

type tokenReviewer struct {
	client client.Client
}

func NewTokenReviewer(client client.Client) IdentityInspector {
	return &tokenReviewer{client: client}
}

func (r *tokenReviewer) WhoAmI(ctx context.Context, token string) (Identity, error) {
	tokenReview := &authv1.TokenReview{
		ObjectMeta: metav1.ObjectMeta{
			Name: "tokenReview",
		},
		Spec: authv1.TokenReviewSpec{
			Token: token,
		},
	}
	err := r.client.Create(ctx, tokenReview)
	if err != nil {
		return Identity{}, fmt.Errorf("failed to create token review: %w", err)
	}

	if !tokenReview.Status.Authenticated {
		return Identity{}, InvalidAuthError{}
	}

	idKind := rbacv1.UserKind
	idName := tokenReview.Status.User.Username

	if isServiceAccount(tokenReview.Status.User) {
		idKind = rbacv1.ServiceAccountKind

		if !strings.HasPrefix(idName, serviceAccountNamePrefix) {
			return Identity{}, fmt.Errorf("invalid serviceaccount name: %q", idName)
		}
		nameSegments := strings.Split(idName, ":")
		idName = nameSegments[len(nameSegments)-1]
	}

	return Identity{
		Name: idName,
		Kind: idKind,
	}, nil
}

func isServiceAccount(subject authv1.UserInfo) bool {
	return contains(subject.Groups, serviceAccountsGroup)
}

func contains(groups []string, soughtGroup string) bool {
	for _, group := range groups {
		if group == soughtGroup {
			return true
		}
	}
	return false
}
