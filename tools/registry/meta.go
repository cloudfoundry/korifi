package registry

import "path"

type ContainerRegistryMeta struct {
	registryBase string
	repoPrefix   string
}

func NewContainerRegistryMeta(registryBase, repoPrefix string) *ContainerRegistryMeta {
	return &ContainerRegistryMeta{
		registryBase: registryBase,
		repoPrefix:   repoPrefix,
	}
}

func (r *ContainerRegistryMeta) PackageRepoName(appGUID string) string {
	return r.repoPrefix + appGUID + "-packages"
}

func (r *ContainerRegistryMeta) PackageImageRef(appGUID string) string {
	return path.Join(r.registryBase, r.PackageRepoName(appGUID))
}

func (r *ContainerRegistryMeta) DropletRepoName(appGUID string) string {
	return r.repoPrefix + appGUID + "-droplets"
}

func (r *ContainerRegistryMeta) DropletImageRef(appGUID string) string {
	return path.Join(r.registryBase, r.DropletRepoName(appGUID))
}
