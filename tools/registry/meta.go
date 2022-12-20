package registry

import (
	"strings"
)

type ContainerRegistryMeta struct {
	registryBase string
	repoPrefix   string
}

func NewContainerRegistryMeta(repoPrefix string) *ContainerRegistryMeta {
	base, prefix, _ := strings.Cut(repoPrefix, "/")
	return &ContainerRegistryMeta{
		registryBase: base,
		repoPrefix:   prefix,
	}
}

func (r *ContainerRegistryMeta) PackageRepoPath(appGUID string) string {
	return r.repoPrefix + appGUID + "-packages"
}

func (r *ContainerRegistryMeta) PackageRepoName(appGUID string) string {
	return r.registryBase + "/" + r.PackageRepoPath(appGUID)
}

func (r *ContainerRegistryMeta) DropletRepoPath(appGUID string) string {
	return r.repoPrefix + appGUID + "-droplets"
}

func (r *ContainerRegistryMeta) DropletRepoName(appGUID string) string {
	return r.registryBase + "/" + r.DropletRepoPath(appGUID)
}
