package repositories

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
}

func NewPackageRepo(
	userClientFactory authorization.UserK8sClientFactory,
	namespaceRetriever NamespaceRetriever,
	authPerms *authorization.NamespacePermissions,
	repositoryCreator RepositoryCreator,
	repositoryPrefix string,
) *PackageRepo {
	return &PackageRepo{
		userClientFactory:    userClientFactory,
		namespaceRetriever:   namespaceRetriever,
		namespacePermissions: authPerms,
		repositoryCreator:    repositoryCreator,
		repositoryPrefix:     repositoryPrefix,
	}
}

type PackageRecord struct {
	GUID        string
	UID         types.UID
	Type        string
	AppGUID     string
	SpaceGUID   string
	State       string
	CreatedAt   string // Can we also just use date objects directly here?
	UpdatedAt   string
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
}

func (message CreatePackageMessage) toCFPackage() korifiv1alpha1.CFPackage {
	guid := uuid.NewString()
	pkg := korifiv1alpha1.CFPackage{
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
	GUID               string
	SpaceGUID          string
	ImageRef           string
	RegistrySecretName string
}

func (r *PackageRepo) CreatePackage(ctx context.Context, authInfo authorization.Info, message CreatePackageMessage) (PackageRecord, error) {
	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed to build user client: %w", err)
	}

	cfPackage := message.toCFPackage()
	err = userClient.Create(ctx, &cfPackage)
	if err != nil {
		return PackageRecord{}, apierrors.FromK8sError(err, PackageResourceType)
	}

	err = r.repositoryCreator.CreateRepository(ctx, r.repositoryRef(message.AppGUID))
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed to create package repository: %w", err)
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

	return r.cfPackageToPackageRecord(*cfPackage), nil
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

	cfPackage := korifiv1alpha1.CFPackage{}
	if err := userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: guid}, &cfPackage); err != nil {
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
		filteredPackages = append(filteredPackages, applyPackageFilter(packageList.Items, message)...)
	}
	return r.convertToPackageRecords(filteredPackages), nil
}

func applyPackageFilter(packages []korifiv1alpha1.CFPackage, message ListPackagesMessage) []korifiv1alpha1.CFPackage {
	var appFiltered []korifiv1alpha1.CFPackage
	if len(message.AppGUIDs) > 0 {
		for _, currentPackage := range packages {
			for _, appGUID := range message.AppGUIDs {
				if currentPackage.Spec.AppRef.Name == appGUID {
					appFiltered = append(appFiltered, currentPackage)
					break
				}
			}
		}
	} else {
		appFiltered = packages
	}

	var stateFiltered []korifiv1alpha1.CFPackage
	if len(message.States) > 0 {
		for _, currentPackage := range appFiltered {
			for _, state := range message.States {
				switch state {
				case PackageStateReady:
					if currentPackage.Spec.Source.Registry.Image != "" {
						stateFiltered = append(stateFiltered, currentPackage)
					}
				case PackageStateAwaitingUpload:
					if currentPackage.Spec.Source.Registry.Image == "" {
						stateFiltered = append(stateFiltered, currentPackage)
					}
				}
			}
		}
	} else {
		stateFiltered = appFiltered
	}

	return stateFiltered
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
		if message.RegistrySecretName != "" {
			imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReference{Name: message.RegistrySecretName})
		}
		cfPackage.Spec.Source.Registry.ImagePullSecrets = imagePullSecrets
	}); err != nil {
		return PackageRecord{}, fmt.Errorf("failed to update package source: %w", apierrors.FromK8sError(err, PackageResourceType))
	}

	record := r.cfPackageToPackageRecord(*cfPackage)
	return record, nil
}

func (r *PackageRepo) cfPackageToPackageRecord(cfPackage korifiv1alpha1.CFPackage) PackageRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfPackage.ObjectMeta)
	state := PackageStateAwaitingUpload
	if cfPackage.Spec.Source.Registry.Image != "" {
		state = PackageStateReady
	}
	return PackageRecord{
		GUID:        cfPackage.Name,
		UID:         cfPackage.UID,
		SpaceGUID:   cfPackage.Namespace,
		Type:        string(cfPackage.Spec.Type),
		AppGUID:     cfPackage.Spec.AppRef.Name,
		State:       state,
		CreatedAt:   formatTimestamp(cfPackage.CreationTimestamp),
		UpdatedAt:   updatedAtTime,
		Labels:      cfPackage.Labels,
		Annotations: cfPackage.Annotations,
		ImageRef:    r.repositoryRef(cfPackage.Spec.AppRef.Name),
	}
}

func (r *PackageRepo) convertToPackageRecords(packages []korifiv1alpha1.CFPackage) []PackageRecord {
	packageRecords := make([]PackageRecord, 0, len(packages))

	for _, currentPackage := range packages {
		packageRecords = append(packageRecords, r.cfPackageToPackageRecord(currentPackage))
	}
	return packageRecords
}

func (r *PackageRepo) repositoryRef(appGUID string) string {
	return r.repositoryPrefix + appGUID + "-packages"
}
