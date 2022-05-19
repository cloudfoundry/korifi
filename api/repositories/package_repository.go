package repositories

import (
	"context"
	"fmt"
	"sort"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/controllers/apis/v1alpha1"

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
}

func NewPackageRepo(
	userClientFactory authorization.UserK8sClientFactory,
	namespaceRetriever NamespaceRetriever,
	authPerms *authorization.NamespacePermissions,
) *PackageRepo {
	return &PackageRepo{
		userClientFactory:    userClientFactory,
		namespaceRetriever:   namespaceRetriever,
		namespacePermissions: authPerms,
	}
}

type PackageRecord struct {
	GUID      string
	UID       types.UID
	Type      string
	AppGUID   string
	SpaceGUID string
	State     string
	CreatedAt string // Can we also just use date objects directly here?
	UpdatedAt string
}

type ListPackagesMessage struct {
	AppGUIDs        []string
	SortBy          string
	DescendingOrder bool
	States          []string
}

type CreatePackageMessage struct {
	Type      string
	AppGUID   string
	SpaceGUID string
	OwnerRef  metav1.OwnerReference
}

func (message CreatePackageMessage) toCFPackage() v1alpha1.CFPackage {
	guid := uuid.NewString()
	return v1alpha1.CFPackage{
		TypeMeta: metav1.TypeMeta{
			Kind:       kind,
			APIVersion: v1alpha1.GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            guid,
			Namespace:       message.SpaceGUID,
			OwnerReferences: []metav1.OwnerReference{message.OwnerRef},
		},
		Spec: v1alpha1.CFPackageSpec{
			Type: v1alpha1.PackageType(message.Type),
			AppRef: corev1.LocalObjectReference{
				Name: message.AppGUID,
			},
		},
	}
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

	return cfPackageToPackageRecord(cfPackage), nil
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

	cfpackage := v1alpha1.CFPackage{}
	if err := userClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: guid}, &cfpackage); err != nil {
		return PackageRecord{}, fmt.Errorf("failed to get package %q: %w", guid, apierrors.FromK8sError(err, PackageResourceType))
	}

	return cfPackageToPackageRecord(cfpackage), nil
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

	var filteredPackages []v1alpha1.CFPackage
	for ns := range nsList {
		packageList := &v1alpha1.CFPackageList{}
		err = userClient.List(ctx, packageList, client.InNamespace(ns))
		if k8serrors.IsForbidden(err) {
			continue
		}
		if err != nil {
			return []PackageRecord{}, fmt.Errorf("failed to list packages in namespace %s: %w", ns, apierrors.FromK8sError(err, PackageResourceType))
		}
		filteredPackages = append(filteredPackages, applyPackageFilter(packageList.Items, message)...)
	}
	orderedPackages := orderPackages(filteredPackages, message)

	return convertToPackageRecords(orderedPackages), nil
}

func orderPackages(packages []v1alpha1.CFPackage, message ListPackagesMessage) []v1alpha1.CFPackage {
	sort.Slice(packages, func(i, j int) bool {
		if message.SortBy == "created_at" && message.DescendingOrder {
			return !packages[i].CreationTimestamp.Before(&packages[j].CreationTimestamp)
		}
		// For now, we order by created_at by default- if you really want to optimize runtime you can use bucketsort
		return packages[i].CreationTimestamp.Before(&packages[j].CreationTimestamp)
	})

	return packages
}

func applyPackageFilter(packages []v1alpha1.CFPackage, message ListPackagesMessage) []v1alpha1.CFPackage {
	var appFiltered []v1alpha1.CFPackage
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

	var stateFiltered []v1alpha1.CFPackage
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
	baseCFPackage := &v1alpha1.CFPackage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      message.GUID,
			Namespace: message.SpaceGUID,
		},
	}
	cfPackage := baseCFPackage.DeepCopy()
	cfPackage.Spec.Source.Registry.Image = message.ImageRef
	cfPackage.Spec.Source.Registry.ImagePullSecrets = []corev1.LocalObjectReference{{Name: message.RegistrySecretName}}

	userClient, err := r.userClientFactory.BuildClient(authInfo)
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed to build user k8s client: %w", err)
	}

	err = userClient.Patch(ctx, cfPackage, client.MergeFrom(baseCFPackage))
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed to update package source: %w", apierrors.FromK8sError(err, PackageResourceType))
	}

	record := cfPackageToPackageRecord(*cfPackage)
	return record, nil
}

func cfPackageToPackageRecord(cfPackage v1alpha1.CFPackage) PackageRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfPackage.ObjectMeta)
	state := PackageStateAwaitingUpload
	if cfPackage.Spec.Source.Registry.Image != "" {
		state = PackageStateReady
	}
	return PackageRecord{
		GUID:      cfPackage.Name,
		UID:       cfPackage.UID,
		SpaceGUID: cfPackage.Namespace,
		Type:      string(cfPackage.Spec.Type),
		AppGUID:   cfPackage.Spec.AppRef.Name,
		State:     state,
		CreatedAt: formatTimestamp(cfPackage.CreationTimestamp),
		UpdatedAt: updatedAtTime,
	}
}

func convertToPackageRecords(packages []v1alpha1.CFPackage) []PackageRecord {
	packageRecords := make([]PackageRecord, 0, len(packages))

	for _, currentPackage := range packages {
		packageRecords = append(packageRecords, cfPackageToPackageRecord(currentPackage))
	}
	return packageRecords
}
