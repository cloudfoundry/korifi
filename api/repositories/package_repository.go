package repositories

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories/conditions"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	kind = "CFPackage"

	PackageStateAwaitingUpload = "AWAITING_UPLOAD"
	PackageStateReady          = "READY"

	PackageResourceType = "Package"
)

type PackageRepo struct {
	userClientFactory    authorization.UserK8sClientFactory
	namespaceRetriever   NamespaceRetriever
	namespacePermissions *authorization.NamespacePermissions
	repositoryCreator    RepositoryCreator
	repositoryPrefix     string
	awaiter              *conditions.Awaiter[*korifiv1alpha1.CFPackage, korifiv1alpha1.CFPackageList, *korifiv1alpha1.CFPackageList]
}

func NewPackageRepo(
	userClientFactory authorization.UserK8sClientFactory,
	namespaceRetriever NamespaceRetriever,
	authPerms *authorization.NamespacePermissions,
	repositoryCreator RepositoryCreator,
	repositoryPrefix string,
	createTimeout time.Duration,
) *PackageRepo {
	return &PackageRepo{
		userClientFactory:    userClientFactory,
		namespaceRetriever:   namespaceRetriever,
		namespacePermissions: authPerms,
		repositoryCreator:    repositoryCreator,
		repositoryPrefix:     repositoryPrefix,
		awaiter:              conditions.NewConditionAwaiter[*korifiv1alpha1.CFPackage, korifiv1alpha1.CFPackageList](createTimeout),
	}
}

type PackageRecord struct {
	GUID        string
	UID         types.UID
	Type        string
	AppGUID     string
	SpaceGUID   string
	State       string
	CreatedAt   time.Time
	UpdatedAt   *time.Time
	Labels      map[string]string
	Annotations map[string]string
	ImageRef    string
}

type ListPackagesMessage struct {
	AppGUIDs []string
	States   []string
}

type CreatePackageMessage struct {
	Type      string
	AppGUID   string
	SpaceGUID string
	Metadata  Metadata
	Data      PackageData
}

type PackageData struct {
	Image    string
	UserName string
	Password string
}

func (message CreatePackageMessage) toCFPackage() *korifiv1alpha1.CFPackage {
	guid := uuid.NewString()
	pkg := &korifiv1alpha1.CFPackage{
		TypeMeta: metav1.TypeMeta{
			Kind:       kind,
			APIVersion: korifiv1alpha1.GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        guid,
			Namespace:   message.SpaceGUID,
			Labels:      message.Metadata.Labels,
			Annotations: message.Metadata.Annotations,
		},
		Spec: korifiv1alpha1.CFPackageSpec{
			Type: korifiv1alpha1.PackageType(message.Type),
			AppRef: corev1.LocalObjectReference{
				Name: message.AppGUID,
			},
		},
	}

	return pkg
}

type UpdatePackageMessage struct {
	GUID          string
	MetadataPatch MetadataPatch
}

type UpdatePackageSourceMessage struct {
	GUID                string
	SpaceGUID           string
	ImageRef            string
	RegistrySecretNames []string
}

func (r *PackageRepo) CreatePackage(ctx context.Context, authInfo authorization.Info, message CreatePackageMessage) (PackageRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfPackage := message.toCFPackage()
	var dockerRegistrySecret *corev1.Secret

	if message.Type == "docker" {
		cfPackage.Spec.Source = korifiv1alpha1.PackageSource{
			Registry: korifiv1alpha1.Registry{
				Image: message.Data.Image,
			},
		}
		if message.Data.UserName != "" && message.Data.Password != "" {
			dockerRegistrySecretName := uuid.NewString()
			var ref name.Reference
			ref, err = name.ParseReference(message.Data.Image)
			if err != nil {
				return PackageRecord{}, fmt.Errorf("failed to parse image ref: %w", err)
			}

			var secretData []byte
			secretData, err = generateDockerCfgJSONContent(message.Data.UserName, message.Data.Password, ref.Context().RegistryStr())
			if err != nil {
				return PackageRecord{}, fmt.Errorf("failed to generate docker registry secret content: %w", err)
			}

			dockerRegistrySecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: message.SpaceGUID,
					Name:      dockerRegistrySecretName,
				},
				Data: map[string][]byte{corev1.DockerConfigJsonKey: secretData},
				Type: corev1.SecretTypeDockerConfigJson,
			}

			err = userClient.Create(ctx, dockerRegistrySecret)
			if err != nil {
				return PackageRecord{}, fmt.Errorf("failed to create docker registry secret: %w", err)
			}

			cfPackage.Spec.Source.Registry.ImagePullSecrets = []corev1.LocalObjectReference{{
				Name: dockerRegistrySecretName,
			}}
		}
	}

	err = userClient.Create(ctx, cfPackage)
	if err != nil {
		return PackageRecord{}, apierrors.FromK8sError(err, PackageResourceType)
	}

	if dockerRegistrySecret != nil {
		err = k8s.PatchResource(ctx, userClient, dockerRegistrySecret, func() {
			_ = controllerutil.SetOwnerReference(cfPackage, dockerRegistrySecret, scheme.Scheme)
		})
		if err != nil {
			return PackageRecord{}, fmt.Errorf("failed to set ownership on the docker registry secret: %v", err)
		}
	}

	err = r.repositoryCreator.CreateRepository(ctx, r.repositoryRef(message.AppGUID))
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed to create package repository: %w", err)
	}

	cfPackage, err = r.awaiter.AwaitCondition(ctx, userClient, cfPackage, workloads.InitializedConditionType)
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed waiting for Initialized condition: %w", err)
	}

	return r.cfPackageToPackageRecord(cfPackage), nil
}

func (r *PackageRepo) UpdatePackage(ctx context.Context, authInfo authorization.Info, updateMessage UpdatePackageMessage) (PackageRecord, error) {
	ns, err := r.namespaceRetriever.NamespaceFor(ctx, updateMessage.GUID, PackageResourceType)
	if err != nil {
		return PackageRecord{}, err
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfPackage := &korifiv1alpha1.CFPackage{}

	err = userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: updateMessage.GUID}, cfPackage)
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed to get package: %w", apierrors.ForbiddenAsNotFound(apierrors.FromK8sError(err, PackageResourceType)))
	}

	err = k8s.PatchResource(ctx, userClient, cfPackage, func() {
		updateMessage.MetadataPatch.Apply(cfPackage)
	})
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed to patch package metadata: %w", apierrors.FromK8sError(err, PackageResourceType))
	}

	return r.cfPackageToPackageRecord(cfPackage), nil
}

func (r *PackageRepo) GetPackage(ctx context.Context, authInfo authorization.Info, guid string) (PackageRecord, error) {
	ns, err := r.namespaceRetriever.NamespaceFor(ctx, guid, PackageResourceType)
	if err != nil {
		return PackageRecord{}, err
	}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed to build user k8s client: %w", err)
	}

	cfPackage := new(korifiv1alpha1.CFPackage)
	if err := userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: guid}, cfPackage); err != nil {
		return PackageRecord{}, fmt.Errorf("failed to get package %q: %w", guid, apierrors.FromK8sError(err, PackageResourceType))
	}

	return r.cfPackageToPackageRecord(cfPackage), nil
}

func (r *PackageRepo) ListPackages(ctx context.Context, authInfo authorization.Info, message ListPackagesMessage) ([]PackageRecord, error) {
	nsList, err := r.namespacePermissions.GetAuthorizedSpaceNamespaces(ctx, authInfo)
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for spaces with user role bindings: %w", err)
	}
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return []PackageRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	preds := []func(korifiv1alpha1.CFPackage) bool{
		SetPredicate(message.AppGUIDs, func(s korifiv1alpha1.CFPackage) string { return s.Spec.AppRef.Name }),
	}
	if len(message.States) > 0 {
		stateSet := NewSet(message.States...)
		preds = append(preds, func(p korifiv1alpha1.CFPackage) bool {
			return (stateSet.Includes(PackageStateReady) && meta.IsStatusConditionTrue(p.Status.Conditions, shared.StatusConditionReady)) ||
				(stateSet.Includes(PackageStateAwaitingUpload) && !meta.IsStatusConditionTrue(p.Status.Conditions, shared.StatusConditionReady))
		})
	}

	var filteredPackages []korifiv1alpha1.CFPackage
	for ns := range nsList {
		packageList := &korifiv1alpha1.CFPackageList{}
		err = userClient.List(ctx, packageList, client.InNamespace(ns))
		if k8serrors.IsForbidden(err) {
			continue
		}
		if err != nil {
			return []PackageRecord{}, fmt.Errorf("failed to list packages in namespace %s: %w", ns, apierrors.FromK8sError(err, PackageResourceType))
		}
		filteredPackages = append(filteredPackages, Filter(packageList.Items, preds...)...)
	}
	return r.convertToPackageRecords(filteredPackages), nil
}

func (r *PackageRepo) UpdatePackageSource(ctx context.Context, authInfo authorization.Info, message UpdatePackageSourceMessage) (PackageRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed to build user k8s client: %w", err)
	}

	cfPackage := &korifiv1alpha1.CFPackage{}
	if err = userClient.Get(ctx, client.ObjectKey{Name: message.GUID, Namespace: message.SpaceGUID}, cfPackage); err != nil {
		return PackageRecord{}, fmt.Errorf("failed to get cf package: %w", apierrors.FromK8sError(err, PackageResourceType))
	}

	if err = k8s.PatchResource(ctx, userClient, cfPackage, func() {
		cfPackage.Spec.Source.Registry.Image = message.ImageRef
		imagePullSecrets := []corev1.LocalObjectReference{}
		for _, secret := range message.RegistrySecretNames {
			imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: secret})
		}
		cfPackage.Spec.Source.Registry.ImagePullSecrets = imagePullSecrets
	}); err != nil {
		return PackageRecord{}, fmt.Errorf("failed to update package source: %w", apierrors.FromK8sError(err, PackageResourceType))
	}

	cfPackage, err = r.awaiter.AwaitCondition(ctx, userClient, cfPackage, shared.StatusConditionReady)
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed awaiting Ready status condition: %w", err)
	}

	record := r.cfPackageToPackageRecord(cfPackage)
	return record, nil
}

func (r *PackageRepo) cfPackageToPackageRecord(cfPackage *korifiv1alpha1.CFPackage) PackageRecord {
	state := PackageStateAwaitingUpload
	if meta.IsStatusConditionTrue(cfPackage.Status.Conditions, shared.StatusConditionReady) {
		state = PackageStateReady
	}
	return PackageRecord{
		GUID:        cfPackage.Name,
		UID:         cfPackage.UID,
		SpaceGUID:   cfPackage.Namespace,
		Type:        string(cfPackage.Spec.Type),
		AppGUID:     cfPackage.Spec.AppRef.Name,
		State:       state,
		CreatedAt:   cfPackage.CreationTimestamp.Time,
		UpdatedAt:   getLastUpdatedTime(cfPackage),
		Labels:      cfPackage.Labels,
		Annotations: cfPackage.Annotations,
		ImageRef:    r.repositoryRef(cfPackage.Spec.AppRef.Name),
	}
}

func (r *PackageRepo) convertToPackageRecords(packages []korifiv1alpha1.CFPackage) []PackageRecord {
	packageRecords := make([]PackageRecord, 0, len(packages))

	for i := range packages {
		packageRecords = append(packageRecords, r.cfPackageToPackageRecord(&packages[i]))
	}
	return packageRecords
}

func (r *PackageRepo) repositoryRef(appGUID string) string {
	return r.repositoryPrefix + appGUID + "-packages"
}

type DockerConfigJSON struct {
	Auths map[string]DockerConfigEntry `json:"auths" datapolicy:"token"`
}

type DockerConfigEntry struct {
	Auth string `json:"auth,omitempty"`
}

func generateDockerCfgJSONContent(username, password, server string) ([]byte, error) {
	if server == "index.docker.io" {
		server = fmt.Sprintf("https://%s/v1/", server)
	}

	dockerConfigAuth := DockerConfigEntry{
		Auth: encodeDockerConfigFieldAuth(username, password),
	}
	dockerConfigJSON := DockerConfigJSON{
		Auths: map[string]DockerConfigEntry{server: dockerConfigAuth},
	}

	return json.Marshal(dockerConfigJSON)
}

// encodeDockerConfigFieldAuth returns base64 encoding of the username and password string
func encodeDockerConfigFieldAuth(username, password string) string {
	fieldValue := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(fieldValue))
}
