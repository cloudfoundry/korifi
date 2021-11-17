package repositories

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/authorization"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"
)

var (
	ErrorDuplicateRoleBinding          = errors.New("RoleBinding with that name already exists")
	ErrorMissingRoleBindingInParentOrg = errors.New("no RoleBinding found in parent org")
)

const (
	RoleGuidLabel         = "cloudfoundry.org/role-guid"
	RoleUserLabel         = "cloudfoundry.org/role-user"
	RoleTypeLabel         = "cloudfoundry.org/role-type"
	roleBindingNamePrefix = "cf"
)

//counterfeiter:generate -o fake -fake-name AuthorizedInChecker . AuthorizedInChecker

type AuthorizedInChecker interface {
	AuthorizedIn(ctx context.Context, identity authorization.Identity, namespace string) (bool, error)
}

//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=create

type RoleRecord struct {
	GUID      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Type      string
	Space     string
	Org       string
	User      string
	Kind      string
}

type RoleRepo struct {
	privilegedClient    client.Client
	roleMappings        map[string]string
	authorizedInChecker AuthorizedInChecker
}

func NewRoleRepo(privilegedClient client.Client, authorizedInChecker AuthorizedInChecker, roleMappings map[string]string) *RoleRepo {
	return &RoleRepo{
		privilegedClient:    privilegedClient,
		roleMappings:        roleMappings,
		authorizedInChecker: authorizedInChecker,
	}
}

func (r *RoleRepo) CreateRole(ctx context.Context, role RoleRecord) (RoleRecord, error) {
	k8sRoleName, ok := r.roleMappings[role.Type]
	if !ok {
		return RoleRecord{}, fmt.Errorf("invalid role type: %q", role.Type)
	}

	userIdentity := authorization.Identity{
		Name: role.User,
		Kind: rbacv1.UserKind,
	}

	if role.Space != "" {
		orgName, err := r.getOrgName(ctx, role.Space)
		if err != nil {
			return RoleRecord{}, err
		}

		hasOrgBinding, err := r.authorizedInChecker.AuthorizedIn(ctx, userIdentity, orgName)
		if err != nil {
			return RoleRecord{}, fmt.Errorf("failed to check for role in parent org: %w", err)
		}

		if !hasOrgBinding {
			return RoleRecord{}, ErrorMissingRoleBindingInParentOrg
		}
	}

	ns := role.Space
	if ns == "" {
		ns = role.Org
	}

	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      calculateRoleBindingName(role),
			Labels: map[string]string{
				RoleGuidLabel: role.GUID,
				RoleUserLabel: role.User,
				RoleTypeLabel: role.Type,
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: role.Kind,
				Name: role.User,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: k8sRoleName,
		},
	}

	err := r.privilegedClient.Create(ctx, &roleBinding)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return RoleRecord{}, ErrorDuplicateRoleBinding
		}
		return RoleRecord{}, fmt.Errorf("failed to assign user %q to role %q: %w", role.User, role.Type, err)
	}

	role.CreatedAt = roleBinding.CreationTimestamp.Time
	role.UpdatedAt = roleBinding.CreationTimestamp.Time

	return role, nil
}

func (r *RoleRepo) getOrgName(ctx context.Context, spaceGUID string) (string, error) {
	namespace := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: spaceGUID,
		},
	}

	err := r.privilegedClient.Get(ctx, client.ObjectKeyFromObject(&namespace), &namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get namespace with name %q: %w", spaceGUID, err)
	}

	orgName := namespace.Annotations[hnsv1alpha2.SubnamespaceOf]
	if orgName == "" {
		return "", fmt.Errorf("namespace %s does not have a parent", spaceGUID)
	}

	return orgName, nil
}

func calculateRoleBindingName(role RoleRecord) string {
	plain := []byte(role.Type + "::" + role.User)
	sum := sha256.Sum256(plain)

	return fmt.Sprintf("%s-%x", roleBindingNamePrefix, sum)
}
