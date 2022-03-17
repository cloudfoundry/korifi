package repositories

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	hnsv1alpha2 "sigs.k8s.io/hierarchical-namespaces/api/v1alpha2"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/config"
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

//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=create

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
	privilegedClient    client.Client
	rootNamespace       string
	roleMappings        map[string]config.Role
	authorizedInChecker AuthorizedInChecker
	userClientFactory   UserK8sClientFactory
}

func NewRoleRepo(privilegedClient client.Client, userClientFactory UserK8sClientFactory, authorizedInChecker AuthorizedInChecker, rootNamespace string, roleMappings map[string]config.Role) *RoleRepo {
	return &RoleRepo{
		privilegedClient:    privilegedClient,
		rootNamespace:       rootNamespace,
		roleMappings:        roleMappings,
		authorizedInChecker: authorizedInChecker,
		userClientFactory:   userClientFactory,
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
		if err = r.validateOrgRequirements(ctx, role, userIdentity); err != nil {
			return RoleRecord{}, err
		}
	}

	ns := role.Space
	if ns == "" {
		ns = role.Org
	}

	annotations := map[string]string{}
	if !k8sRoleConfig.Propagate {
		annotations[hnsv1alpha2.AnnotationNoneSelector] = "true"
	}

	roleBinding := createRoleBinding(ns, role.Type, role.Kind, role.User, role.GUID, k8sRoleConfig.Name, annotations)

	err = userClient.Create(ctx, &roleBinding)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return RoleRecord{}, apierrors.NewUnprocessableEntityError(
				fmt.Errorf("rolebinging %s:%s already exists", roleBinding.Namespace, roleBinding.Name),
				"RoleBinding with that name already exists",
			)
		}
		return RoleRecord{}, fmt.Errorf("failed to assign user %q to role %q: %w", role.User, role.Type, apierrors.FromK8sError(err, RoleResourceType))
	}

	cfUserk8sRoleConfig, ok := r.roleMappings[cfUserRoleType]
	if !ok {
		return RoleRecord{}, fmt.Errorf("invalid role type: %q", cfUserRoleType)
	}

	cfUserRoleBinding := createRoleBinding(r.rootNamespace, cfUserRoleType, role.Kind, role.User, uuid.NewString(), cfUserk8sRoleConfig.Name, annotations)
	err = r.privilegedClient.Create(ctx, &cfUserRoleBinding)
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

func (r *RoleRepo) validateOrgRequirements(ctx context.Context, role CreateRoleMessage, userIdentity authorization.Identity) error {
	orgName, err := r.getOrgName(ctx, role.Space)
	if err != nil {
		return err
	}

	hasOrgBinding, err := r.authorizedInChecker.AuthorizedIn(ctx, userIdentity, orgName)
	if err != nil {
		return fmt.Errorf("failed to check for role in parent org: %w", err)
	}

	if !hasOrgBinding {
		return apierrors.NewUnprocessableEntityError(
			fmt.Errorf("no RoleBinding found in parent org %q for user %q", orgName, userIdentity.Name),
			"no RoleBinding found in parent org",
		)
	}
	return nil
}

func (r *RoleRepo) getOrgName(ctx context.Context, spaceGUID string) (string, error) {
	namespace := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: spaceGUID,
		},
	}

	err := r.privilegedClient.Get(ctx, client.ObjectKeyFromObject(&namespace), &namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get namespace with name %q: %w", spaceGUID, apierrors.FromK8sError(err, OrgResourceType))
	}

	orgName := namespace.Annotations[hnsv1alpha2.SubnamespaceOf]
	if orgName == "" {
		return "", fmt.Errorf("namespace %s does not have a parent", spaceGUID)
	}

	return orgName, nil
}

func calculateRoleBindingName(roleType, roleUser string) string {
	plain := []byte(roleType + "::" + roleUser)
	sum := sha256.Sum256(plain)

	return fmt.Sprintf("%s-%x", roleBindingNamePrefix, sum)
}

func createRoleBinding(namespace, roleType, roleKind, roleUser, roleGUID, roleConfigName string, annotations map[string]string) rbacv1.RoleBinding {
	return rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      calculateRoleBindingName(roleType, roleUser),
			Labels: map[string]string{
				RoleGuidLabel: roleGUID,
			},
			Annotations: annotations,
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
