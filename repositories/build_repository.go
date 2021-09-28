package repositories

import (
	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	BuildStateStaging = "STAGING"
	BuildStateStaged  = "STAGED"
	BuildStateFailed  = "FAILED"
)

type BuildRecord struct {
	GUID            string
	State           string
	CreatedAt       string
	UpdatedAt       string
	StagingMemoryMB int
	StagingDiskMB   int
	Lifecycle       Lifecycle
	PackageGUID     string
	DropletGUID     string
	AppGUID         string
	Labels          map[string]string
	Annotations     map[string]string
}

type BuildRepo struct {
}

func (b *BuildRepo) FetchBuild(ctx context.Context, k8sClient client.Client, buildGUID string) (BuildRecord, error) {
	// TODO: Could look up namespace from guid => namespace cache to do Get
	buildList := &workloadsv1alpha1.CFBuildList{}
	err := k8sClient.List(ctx, buildList)
	if err != nil { // untested
		return BuildRecord{}, err
	}
	allBuilds := buildList.Items
	matches := b.filterBuildsByMetadataName(allBuilds, buildGUID)

	return b.returnBuild(matches)
}

func (b *BuildRepo) returnBuild(builds []workloadsv1alpha1.CFBuild) (BuildRecord, error) {
	if len(builds) == 0 {
		return BuildRecord{}, NotFoundError{}
	}
	if len(builds) > 1 {
		return BuildRecord{}, errors.New("duplicate builds exist")
	}

	return b.cfBuildToBuildRecord(builds[0]), nil
}

func (b *BuildRepo) cfBuildToBuildRecord(cfBuild workloadsv1alpha1.CFBuild) BuildRecord {
	updatedAtTime, _ := getTimeLastUpdatedTimestamp(&cfBuild.ObjectMeta)

	// TODO: Fill in more fields !!!

	return BuildRecord{
		GUID:        cfBuild.Name,
		Labels:      cfBuild.Labels,
		Annotations: cfBuild.Annotations,
		Lifecycle: Lifecycle{
			Data: LifecycleData{
				Buildpacks: cfBuild.Spec.Lifecycle.Data.Buildpacks,
				Stack:      cfBuild.Spec.Lifecycle.Data.Stack,
			},
		},
		CreatedAt: cfBuild.CreationTimestamp.UTC().Format(TimestampFormat),
		UpdatedAt: updatedAtTime,
	}
}

func (b *BuildRepo) filterBuildsByMetadataName(builds []workloadsv1alpha1.CFBuild, name string) []workloadsv1alpha1.CFBuild {
	var filtered []workloadsv1alpha1.CFBuild
	for i, build := range builds {
		if build.ObjectMeta.Name == name {
			filtered = append(filtered, builds[i])
		}
	}
	return filtered
}
