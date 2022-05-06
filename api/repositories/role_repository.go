package repositories

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/config"
)

const (
	PropagateCfRoleLabel  = "cloudfoundry.org/propagate-cf-role"
	RoleGuidLabel         = "cloudfoundry.org/role-guid"
	roleBindingNamePrefix = "cf"
	cfUserRoleType        = "cf_user"
	RoleResourceType      = "Role"
)

//counterfeiter:generate -o fake -fake-name AuthorizedInChecker . AuthorizedInChecker

type AuthorizedInChecker interface {
	AuthorizedIn(ctx context.Context, identity authorization.Identity, namespace string) (bool, error)
}

type CreateRoleMessage struct {
	GUID  string
	Type  string
	Space string
	Org   string
	User  string
	Kind  string
}

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
	rootNamespace       string
	roleMappings        map[string]config.Role
	authorizedInChecker AuthorizedInChecker
	userClientFactory   UserK8sClientFactory
	spaceRepo           *SpaceRepo
}

func NewRoleRepo(userClientFactory UserK8sClientFactory, spaceRepo *SpaceRepo, authorizedInChecker AuthorizedInChecker, rootNamespace string, roleMappings map[string]config.Role) *RoleRepo {
	return &RoleRepo{
		rootNamespace:       rootNamespace,
		roleMappings:        roleMappings,
		authorizedInChecker: authorizedInChecker,
		userClientFactory:   userClientFactory,
		spaceRepo:           spaceRepo,
	}
}

func (r *RoleRepo) CreateRole(ctx context.Context, authInfo authorization.Info, role CreateRoleMessage) (RoleRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return RoleRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	k8sRoleConfig, ok := r.roleMappings[role.Type]
	if !ok {
		return RoleRecord{}, fmt.Errorf("invalid role type: %q", role.Type)
	}

	userIdentity := authorization.Identity{
		Name: role.User,
		Kind: role.Kind,
	}

	if role.Space != "" {
		if err = r.validateOrgRequirements(ctx, role, userIdentity, authInfo); err != nil {
			return RoleRecord{}, err
		}
	}

	ns := role.Space
	if ns == "" {
		ns = role.Org
	}

	roleBinding := createRoleBinding(ns, role.Type, role.Kind, role.User, role.GUID, k8sRoleConfig.Name, k8sRoleConfig.Propagate)

	if !k8sRoleConfig.Propagate {

	}

	err = userClient.Create(ctx, &roleBinding)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			errorDetail := fmt.Sprintf("User '%s' already has '%s' role", role.User, role.Type)
			return RoleRecord{}, apierrors.NewUnprocessableEntityError(
				fmt.Errorf("rolebinding %s:%s already exists", roleBinding.Namespace, roleBinding.Name),
				errorDetail,
			)
		}
		return RoleRecord{}, fmt.Errorf("failed to assign user %q to role %q: %w", role.User, role.Type, apierrors.FromK8sError(err, RoleResourceType))
	}

	cfUserk8sRoleConfig, ok := r.roleMappings[cfUserRoleType]
	if !ok {
		return RoleRecord{}, fmt.Errorf("invalid role type: %q", cfUserRoleType)
	}

	cfUserRoleBinding := createRoleBinding(r.rootNamespace, cfUserRoleType, role.Kind, role.User, uuid.NewString(), cfUserk8sRoleConfig.Name, k8sRoleConfig.Propagate)
	err = userClient.Create(ctx, &cfUserRoleBinding)
	if err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return RoleRecord{}, fmt.Errorf("failed to assign user %q to role %q: %w", role.User, role.Type, err)
		}
	}

	roleRecord := RoleRecord{
		GUID:      role.GUID,
		CreatedAt: roleBinding.CreationTimestamp.Time,
		UpdatedAt: roleBinding.CreationTimestamp.Time,
		Type:      role.Type,
		Space:     role.Space,
		Org:       role.Org,
		User:      role.User,
		Kind:      role.Kind,
	}

	return roleRecord, nil
}

func (r *RoleRepo) validateOrgRequirements(ctx context.Context, role CreateRoleMessage, userIdentity authorization.Identity, authInfo authorization.Info) error {
	space, err := r.spaceRepo.GetSpace(ctx, authInfo, role.Space)
	if err != nil {
		return apierrors.AsUnprocessableEntity(err, "space not found", apierrors.NotFoundError{}, apierrors.ForbiddenError{})
	}

	hasOrgBinding, err := r.authorizedInChecker.AuthorizedIn(ctx, userIdentity, space.OrganizationGUID)
	if err != nil {
		return fmt.Errorf("failed to check for role in parent org: %w", err)
	}

	if !hasOrgBinding {
		return apierrors.NewUnprocessableEntityError(
			fmt.Errorf("no RoleBinding found in parent org %q for user %q", space.OrganizationGUID, userIdentity.Name),
			"no RoleBinding found in parent org",
		)
	}
	return nil
}

func calculateRoleBindingName(roleType, roleUser string) string {
	plain := []byte(roleType + "::" + roleUser)
	sum := sha256.Sum256(plain)

	return fmt.Sprintf("%s-%x", roleBindingNamePrefix, sum)
}

func createRoleBinding(namespace, roleType, roleKind, roleUser, roleGUID, roleConfigName string, propagateLabelValue bool) rbacv1.RoleBinding {
	return rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      calculateRoleBindingName(roleType, roleUser),
			Labels: map[string]string{
				PropagateCfRoleLabel: strconv.FormatBool(propagateLabelValue),
				RoleGuidLabel:        roleGUID,
			},
			Annotations: map[string]string{},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: roleKind,
				Name: roleUser,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: roleConfigName,
		},
	}
}
