package repositories

import (
	"context"
	"errors"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	kind = "CFPackage"
)

type PackageCreate struct {
	Type      string
	AppGUID   string
	SpaceGUID string
}

type PackageRecord struct {
	GUID      string
	Type      string
	AppGUID   string
	State     string
	CreatedAt string
	UpdatedAt string
}

type PackageRepo struct{}

func (r *PackageRepo) CreatePackage(ctx context.Context, client client.Client, cp PackageCreate) (PackageRecord, error) {
	cfPackage := r.packageCreateToCFPackage(cp)
	err := client.Create(ctx, &cfPackage)
	if err != nil {
		return PackageRecord{}, err
	}
	return r.cfPackageToPackageRecord(cfPackage), nil
}

func (r *PackageRepo) FetchPackage(ctx context.Context, client client.Client, guid string) (PackageRecord, error) {
	packageList := &workloadsv1alpha1.CFPackageList{}
	err := client.List(ctx, packageList)
	if err != nil { // untested
		return PackageRecord{}, err
	}
	allPackages := packageList.Items
	matches := r.filterPackagesByMetadataName(allPackages, guid)

	return r.returnPackage(matches)
}

func (r *PackageRepo) packageCreateToCFPackage(cp PackageCreate) workloadsv1alpha1.CFPackage {
	guid := uuid.New().String()
	return workloadsv1alpha1.CFPackage{
		TypeMeta: metav1.TypeMeta{
			Kind:       kind,
			APIVersion: workloadsv1alpha1.GroupVersion.Identifier(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      guid,
			Namespace: cp.SpaceGUID,
		},
		Spec: workloadsv1alpha1.CFPackageSpec{
			Type: workloadsv1alpha1.PackageType(cp.Type),
			AppRef: workloadsv1alpha1.ResourceReference{
				Name: cp.AppGUID,
			},
		},
	}
}

func (r *PackageRepo) cfPackageToPackageRecord(cfPackage workloadsv1alpha1.CFPackage) PackageRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfPackage.ObjectMeta)
	return PackageRecord{
		GUID:      cfPackage.ObjectMeta.Name,
		Type:      string(cfPackage.Spec.Type),
		AppGUID:   cfPackage.Spec.AppRef.Name,
		State:     "AWAITING_UPLOAD",
		CreatedAt: formatTimestamp(cfPackage.CreationTimestamp),
		UpdatedAt: updatedAtTime,
	}
}

func (r *PackageRepo) filterPackagesByMetadataName(packages []workloadsv1alpha1.CFPackage, name string) []workloadsv1alpha1.CFPackage {
	var filtered []workloadsv1alpha1.CFPackage
	for i, app := range packages {
		if app.ObjectMeta.Name == name {
			filtered = append(filtered, packages[i])
		}
	}
	return filtered
}

func (r *PackageRepo) returnPackage(apps []workloadsv1alpha1.CFPackage) (PackageRecord, error) {
	if len(apps) == 0 {
		return PackageRecord{}, NotFoundError{}
	}
	if len(apps) > 1 {
		return PackageRecord{}, errors.New("duplicate packages exist")
	}

	return r.cfPackageToPackageRecord(apps[0]), nil
}
