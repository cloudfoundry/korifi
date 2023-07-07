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
	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/config"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
)

const (
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
	GUID                    string
	Type                    string
	Space                   string
	Org                     string
	User                    string
	Kind                    string
	ServiceAccountNamespace string
}

type DeleteRoleMessage struct {
	GUID  string
	Space string
	Org   string
}

type RoleRecord struct {
	GUID      string
	CreatedAt time.Time
	UpdatedAt *time.Time
	Type      string
	Space     string
	Org       string
	User      string
	Kind      string
}

type RoleRepo struct {
	rootNamespace        string
	roleMappings         map[string]config.Role
	inverseRoleMappings  map[string]string
	authorizedInChecker  AuthorizedInChecker
	namespacePermissions *authorization.NamespacePermissions
	userClientFactory    authorization.UserK8sClientFactory
	spaceRepo            *SpaceRepo
	namespaceRetriever   NamespaceRetriever
}

func NewRoleRepo(
	userClientFactory authorization.UserK8sClientFactory,
	spaceRepo *SpaceRepo,
	authorizedInChecker AuthorizedInChecker,
	namespacePermissions *authorization.NamespacePermissions,
	rootNamespace string,
	roleMappings map[string]config.Role,
	namespaceRetriever NamespaceRetriever,
) *RoleRepo {
	inverseRoleMappings := map[string]string{}
	for k, v := range roleMappings {
		inverseRoleMappings[v.Name] = k
	}

	return &RoleRepo{
		rootNamespace:        rootNamespace,
		roleMappings:         roleMappings,
		inverseRoleMappings:  inverseRoleMappings,
		authorizedInChecker:  authorizedInChecker,
		namespacePermissions: namespacePermissions,
		userClientFactory:    userClientFactory,
		spaceRepo:            spaceRepo,
		namespaceRetriever:   namespaceRetriever,
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

	if role.Kind == rbacv1.ServiceAccountKind {
		userIdentity.Name = fmt.Sprintf("system:serviceaccount:%s:%s", role.ServiceAccountNamespace, role.User)
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

	roleBinding := createRoleBinding(ns, role.Type, role.Kind, role.User, role.ServiceAccountNamespace, role.GUID, k8sRoleConfig.Name, k8sRoleConfig.Propagate)

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

	cfUserRoleBinding := createRoleBinding(r.rootNamespace, cfUserRoleType, role.Kind, role.User, role.ServiceAccountNamespace, uuid.NewString(), cfUserk8sRoleConfig.Name, false)
	err = userClient.Create(ctx, &cfUserRoleBinding)
	if err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return RoleRecord{}, fmt.Errorf("failed to assign user %q to role %q: %w", role.User, role.Type, err)
		}
	}

	roleRecord := RoleRecord{
		GUID:      role.GUID,
		CreatedAt: roleBinding.CreationTimestamp.Time,
		UpdatedAt: &roleBinding.CreationTimestamp.Time,
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

func calculateRoleBindingName(roleType, roleServiceAccountNamespace, roleUser string) string {
	roleBindingName := roleType + "::"
	if roleServiceAccountNamespace != "" {
		roleBindingName = roleBindingName + roleServiceAccountNamespace + "/"
	}
	roleBindingName = roleBindingName + roleUser
	plain := []byte(roleBindingName)
	sum := sha256.Sum256(plain)

	return fmt.Sprintf("%s-%x", roleBindingNamePrefix, sum)
}

func createRoleBinding(namespace, roleType, roleKind, roleUser, roleServiceAccountNamespace, roleGUID, roleConfigName string, propagateLabelValue bool) rbacv1.RoleBinding {
	return rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      calculateRoleBindingName(roleType, roleServiceAccountNamespace, roleUser),
			Labels: map[string]string{
				RoleGuidLabel: roleGUID,
			},
			Annotations: map[string]string{
				korifiv1alpha1.PropagateRoleBindingAnnotation: strconv.FormatBool(propagateLabelValue),
			},
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      roleKind,
				Name:      roleUser,
				Namespace: roleServiceAccountNamespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "ClusterRole",
			Name: roleConfigName,
		},
	}
}

func (r *RoleRepo) ListRoles(ctx context.Context, authInfo authorization.Info) ([]RoleRecord, error) {
	spaceList, err := r.namespacePermissions.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}
	orgList, err := r.namespacePermissions.GetAuthorizedOrgNamespaces(ctx, authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}

	var nsList []string
	for k := range spaceList {
		nsList = append(nsList, k)
	}
	for k := range orgList {
		nsList = append(nsList, k)
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to build user client: %w", err)
	}

	var roles []RoleRecord
	for _, ns := range nsList {
		roleBindings := &rbacv1.RoleBindingList{}
		err := userClient.List(ctx, roleBindings, client.InNamespace(ns))
		if err != nil {
			if k8serrors.IsForbidden(err) {
				continue
			}
			return nil, fmt.Errorf("failed to list roles in namespace %s: %w", ns, apierrors.FromK8sError(err, RoleResourceType))
		}

		for _, roleBinding := range roleBindings.Items {
			if roleBinding.Labels[korifiv1alpha1.PropagatedFromLabel] != "" {
				continue
			}

			cfRoleName := r.inverseRoleMappings[roleBinding.RoleRef.Name]
			if cfRoleName == "" {
				continue
			}

			roles = append(roles, r.toRoleRecord(roleBinding, cfRoleName))
		}
	}

	return roles, nil
}

func (r *RoleRepo) GetRole(ctx context.Context, authInfo authorization.Info, roleGUID string) (RoleRecord, error) {
	roles, err := r.ListRoles(ctx, authInfo)
	if err != nil {
		return RoleRecord{}, err
	}

	matchedRoles := []RoleRecord{}
	for _, role := range roles {
		if role.GUID == roleGUID {
			matchedRoles = append(matchedRoles, role)
		}
	}

	switch len(matchedRoles) {
	case 0:
		return RoleRecord{}, apierrors.NewNotFoundError(nil, RoleResourceType)
	case 1:
		return matchedRoles[0], nil
	default:
		return RoleRecord{}, fmt.Errorf("multiple role bindings with guid %q found", roleGUID)
	}
}

func (r *RoleRepo) DeleteRole(ctx context.Context, authInfo authorization.Info, deleteMsg DeleteRoleMessage) error {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return fmt.Errorf("failed to build user client: %w", err)
	}

	ns := deleteMsg.Org
	if ns == "" {
		ns = deleteMsg.Space
	}

	roleBindings := &rbacv1.RoleBindingList{}
	err = userClient.List(ctx, roleBindings, client.InNamespace(ns), client.MatchingLabels{
		RoleGuidLabel: deleteMsg.GUID,
	})
	if err != nil {
		return fmt.Errorf("failed to list roles with guid %q in namespace %q: %w", deleteMsg.GUID, ns, apierrors.FromK8sError(err, RoleResourceType))
	}

	switch len(roleBindings.Items) {
	case 0:
		return apierrors.NewNotFoundError(nil, RoleResourceType)
	case 1:
		rb := &roleBindings.Items[0]
		err := userClient.Delete(ctx, rb)
		if err != nil {
			return fmt.Errorf("failed to delete role binding %s/%s: %w", rb.Namespace, rb.Name, apierrors.FromK8sError(err, RoleResourceType))
		}
	default:
		return fmt.Errorf("multiple role bindings with guid %q found", deleteMsg.GUID)
	}

	return nil
}

func (r *RoleRepo) toRoleRecord(roleBinding rbacv1.RoleBinding, cfRoleName string) RoleRecord {
	record := RoleRecord{
		GUID:      roleBinding.Labels[RoleGuidLabel],
		CreatedAt: roleBinding.CreationTimestamp.Time,
		UpdatedAt: getLastUpdatedTime(&roleBinding),
		Type:      cfRoleName,
		User:      roleBinding.Subjects[0].Name,
		Kind:      roleBinding.Subjects[0].Kind,
	}

	if record.Kind == rbacv1.ServiceAccountKind {
		record.User = fmt.Sprintf("system:serviceaccount:%s:%s", roleBinding.Subjects[0].Namespace, roleBinding.Subjects[0].Name)
	}

	switch r.roleMappings[cfRoleName].Level {
	case config.OrgRole:
		record.Org = roleBinding.Namespace
	case config.SpaceRole:
		record.Space = roleBinding.Namespace
	}

	return record
}
