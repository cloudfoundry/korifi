package repositories

import (
	"context"
	"fmt"
	"slices"
	"time"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories/compare"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/packages"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/dockercfg"

	"github.com/BooleanCat/go-functional/v2/it"
	"github.com/BooleanCat/go-functional/v2/it/itx"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	kind = "CFPackage"

	PackageStateAwaitingUpload = "AWAITING_UPLOAD"
	PackageStateReady          = "READY"

	PackageResourceType = "Package"
)

var packageTypeToLifecycleType = map[korifiv1alpha1.PackageType]korifiv1alpha1.LifecycleType{
	"bits":   "buildpack",
	"docker": "docker",
}

type PackageRepo struct {
	klient            Klient
	repositoryCreator RepositoryCreator
	repositoryPrefix  string
	awaiter           Awaiter[*korifiv1alpha1.CFPackage]
	sorter            PackageSorter
}

func NewPackageRepo(
	klient Klient,
	repositoryCreator RepositoryCreator,
	repositoryPrefix string,
	awaiter Awaiter[*korifiv1alpha1.CFPackage],
	sorter PackageSorter,
) *PackageRepo {
	return &PackageRepo{
		klient:            klient,
		repositoryCreator: repositoryCreator,
		repositoryPrefix:  repositoryPrefix,
		awaiter:           awaiter,
		sorter:            sorter,
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

func (r PackageRecord) Relationships() map[string]string {
	return map[string]string{
		"app": r.AppGUID,
	}
}

//counterfeiter:generate -o fake -fake-name PackageSorter . PackageSorter
type PackageSorter interface {
	Sort(records []PackageRecord, order string) []PackageRecord
}

type packageSorter struct {
	sorter *compare.Sorter[PackageRecord]
}

func NewPackageSorter() *packageSorter {
	return &packageSorter{
		sorter: compare.NewSorter(PackageComparator),
	}
}

func (s *packageSorter) Sort(records []PackageRecord, order string) []PackageRecord {
	return s.sorter.Sort(records, order)
}

func PackageComparator(fieldName string) func(PackageRecord, PackageRecord) int {
	return func(d1, d2 PackageRecord) int {
		switch fieldName {
		case "created_at":
			return tools.CompareTimePtr(&d1.CreatedAt, &d2.CreatedAt)
		case "-created_at":
			return tools.CompareTimePtr(&d2.CreatedAt, &d1.CreatedAt)
		case "updated_at":
			return tools.CompareTimePtr(d1.UpdatedAt, d2.UpdatedAt)
		case "-updated_at":
			return tools.CompareTimePtr(d2.UpdatedAt, d1.UpdatedAt)
		}
		return 0
	}
}

type ListPackagesMessage struct {
	GUIDs    []string
	AppGUIDs []string
	States   []string
	OrderBy  string
}

func (m *ListPackagesMessage) matches(p korifiv1alpha1.CFPackage) bool {
	return tools.EmptyOrContains(m.GUIDs, p.Name) &&
		tools.EmptyOrContains(m.AppGUIDs, p.Spec.AppRef.Name) &&
		m.matchesState(p)
}

func (m *ListPackagesMessage) matchesState(p korifiv1alpha1.CFPackage) bool {
	if len(m.States) == 0 {
		return true
	}

	if slices.Contains(m.States, PackageStateReady) && meta.IsStatusConditionTrue(p.Status.Conditions, korifiv1alpha1.StatusConditionReady) {
		return true
	}

	if slices.Contains(m.States, PackageStateAwaitingUpload) && !meta.IsStatusConditionTrue(p.Status.Conditions, korifiv1alpha1.StatusConditionReady) {
		return true
	}

	return false
}

type CreatePackageMessage struct {
	Type      string
	AppGUID   string
	SpaceGUID string
	Metadata  Metadata
	Data      *PackageData
}

type PackageData struct {
	Image    string
	Username *string
	Password *string
}

func (message CreatePackageMessage) toCFPackage() *korifiv1alpha1.CFPackage {
	pkg := &korifiv1alpha1.CFPackage{
		TypeMeta: metav1.TypeMeta{
			Kind:       kind,
			APIVersion: korifiv1alpha1.SchemeGroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        uuid.NewString(),
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

	if message.Type == "docker" {
		pkg.Spec.Source.Registry.Image = message.Data.Image
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
	cfApp := &korifiv1alpha1.CFApp{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: message.SpaceGUID,
			Name:      message.AppGUID,
		},
	}

	err := r.klient.Get(ctx, cfApp)
	if err != nil {
		return PackageRecord{},
			apierrors.AsUnprocessableEntity(
				apierrors.FromK8sError(err, ServiceBindingResourceType),
				"Referenced app not found. Ensure that the app exists and you have access to it.",
				apierrors.ForbiddenError{},
				apierrors.NotFoundError{},
			)
	}

	cfPackage := message.toCFPackage()
	err = r.klient.Create(ctx, cfPackage)
	if err != nil {
		return PackageRecord{}, apierrors.FromK8sError(err, PackageResourceType)
	}

	if packageTypeToLifecycleType[cfPackage.Spec.Type] != cfApp.Spec.Lifecycle.Type {
		return PackageRecord{}, apierrors.NewUnprocessableEntityError(nil, fmt.Sprintf("cannot create %s package for a %s app", cfPackage.Spec.Type, cfApp.Spec.Lifecycle.Type))
	}

	if cfPackage.Spec.Type == "bits" {
		err = r.repositoryCreator.CreateRepository(ctx, r.repositoryRef(*cfPackage))
		if err != nil {
			return PackageRecord{}, fmt.Errorf("failed to create package repository: %w", err)
		}
	}

	if isPrivateDockerImage(message) {
		err = r.createImagePullSecret(ctx, cfPackage, message)
		if err != nil {
			return PackageRecord{}, fmt.Errorf("failed to build docker image pull secret: %w", err)
		}
	}

	cfPackage, err = r.awaiter.AwaitCondition(ctx, r.klient, cfPackage, packages.InitializedConditionType)
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed waiting for Initialized condition: %w", err)
	}

	return r.cfPackageToPackageRecord(*cfPackage), nil
}

func isPrivateDockerImage(message CreatePackageMessage) bool {
	return message.Type == "docker" &&
		message.Data.Username != nil &&
		message.Data.Password != nil
}

func (r *PackageRepo) createImagePullSecret(ctx context.Context, cfPackage *korifiv1alpha1.CFPackage, message CreatePackageMessage) error {
	ref, err := name.ParseReference(message.Data.Image)
	if err != nil {
		return fmt.Errorf("failed to parse image ref: %w", err)
	}

	imgPullSecret, err := dockercfg.CreateDockerConfigSecret(
		cfPackage.Namespace,
		cfPackage.Name,
		dockercfg.DockerServerConfig{
			Server:   ref.Context().RegistryStr(),
			Username: *message.Data.Username,
			Password: *message.Data.Password,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to generate image pull secret: %w", err)
	}

	err = controllerutil.SetOwnerReference(cfPackage, imgPullSecret, scheme.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set ownership from the package to the image pull secret: %w", err)
	}

	err = r.klient.Create(ctx, imgPullSecret)
	if err != nil {
		return fmt.Errorf("failed create the image pull secret: %w", err)
	}

	err = r.klient.Patch(ctx, cfPackage, func() error {
		cfPackage.Spec.Source.Registry.ImagePullSecrets = []corev1.LocalObjectReference{{Name: imgPullSecret.Name}}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed set the package image pull secret: %w", err)
	}

	return nil
}

func (r *PackageRepo) UpdatePackage(ctx context.Context, authInfo authorization.Info, updateMessage UpdatePackageMessage) (PackageRecord, error) {
	cfPackage := &korifiv1alpha1.CFPackage{
		ObjectMeta: metav1.ObjectMeta{
			Name: updateMessage.GUID,
		},
	}

	err := r.klient.Get(ctx, cfPackage)
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed to get package: %w", apierrors.ForbiddenAsNotFound(apierrors.FromK8sError(err, PackageResourceType)))
	}

	err = r.klient.Patch(ctx, cfPackage, func() error {
		updateMessage.MetadataPatch.Apply(cfPackage)

		return nil
	})
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed to patch package metadata: %w", apierrors.FromK8sError(err, PackageResourceType))
	}

	return r.cfPackageToPackageRecord(*cfPackage), nil
}

func (r *PackageRepo) GetPackage(ctx context.Context, authInfo authorization.Info, guid string) (PackageRecord, error) {
	cfPackage := &korifiv1alpha1.CFPackage{
		ObjectMeta: metav1.ObjectMeta{
			Name: guid,
		},
	}
	if err := r.klient.Get(ctx, cfPackage); err != nil {
		return PackageRecord{}, fmt.Errorf("failed to get package %q: %w", guid, apierrors.FromK8sError(err, PackageResourceType))
	}

	return r.cfPackageToPackageRecord(*cfPackage), nil
}

func (r *PackageRepo) ListPackages(ctx context.Context, authInfo authorization.Info, message ListPackagesMessage) ([]PackageRecord, error) {
	packageList := &korifiv1alpha1.CFPackageList{}
	_, err := r.klient.List(ctx, packageList)
	if err != nil {
		return []PackageRecord{}, fmt.Errorf("failed to list packages: %w", apierrors.FromK8sError(err, PackageResourceType))
	}

	filteredPackages := itx.FromSlice(packageList.Items).Filter(message.matches)
	return r.sorter.Sort(slices.Collect(it.Map(filteredPackages, r.cfPackageToPackageRecord)), message.OrderBy), nil
}

func (r *PackageRepo) UpdatePackageSource(ctx context.Context, authInfo authorization.Info, message UpdatePackageSourceMessage) (PackageRecord, error) {
	cfPackage := &korifiv1alpha1.CFPackage{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: message.SpaceGUID,
			Name:      message.GUID,
		},
	}
	if err := r.klient.Get(ctx, cfPackage); err != nil {
		return PackageRecord{}, fmt.Errorf("failed to get cf package: %w", apierrors.FromK8sError(err, PackageResourceType))
	}

	err := r.klient.Patch(ctx, cfPackage, func() error {
		cfPackage.Spec.Source.Registry.Image = message.ImageRef
		cfPackage.Spec.Source.Registry.ImagePullSecrets = slices.Collect(
			it.Map(slices.Values(message.RegistrySecretNames), func(secret string) corev1.LocalObjectReference {
				return corev1.LocalObjectReference{Name: secret}
			}),
		)
		return nil
	})
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed to update package source: %w", apierrors.FromK8sError(err, PackageResourceType))
	}

	cfPackage, err = r.awaiter.AwaitCondition(ctx, r.klient, cfPackage, korifiv1alpha1.StatusConditionReady)
	if err != nil {
		return PackageRecord{}, fmt.Errorf("failed awaiting Ready status condition: %w", err)
	}

	record := r.cfPackageToPackageRecord(*cfPackage)
	return record, nil
}

func (r *PackageRepo) cfPackageToPackageRecord(cfPackage korifiv1alpha1.CFPackage) PackageRecord {
	state := PackageStateAwaitingUpload
	if meta.IsStatusConditionTrue(cfPackage.Status.Conditions, korifiv1alpha1.StatusConditionReady) {
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
		UpdatedAt:   getLastUpdatedTime(&cfPackage),
		Labels:      cfPackage.Labels,
		Annotations: cfPackage.Annotations,
		ImageRef:    r.repositoryRef(cfPackage),
	}
}

func (r *PackageRepo) repositoryRef(cfPackage korifiv1alpha1.CFPackage) string {
	if cfPackage.Spec.Type == "docker" {
		return cfPackage.Spec.Source.Registry.Image
	}

	return r.repositoryPrefix + cfPackage.Spec.AppRef.Name + "-packages"
}
