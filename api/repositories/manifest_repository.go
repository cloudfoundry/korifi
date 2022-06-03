package repositories

type ManifestRepo struct {
	*SpaceRepo
	*AppRepo
	*DomainRepo
	*ProcessRepo
	*RouteRepo
}

func NewManifestRepo(spaceRepo *SpaceRepo, appRepo *AppRepo, domainRepo *DomainRepo, processRepo *ProcessRepo, routeRepo *RouteRepo) *ManifestRepo {
	return &ManifestRepo{
		SpaceRepo:   spaceRepo,
		AppRepo:     appRepo,
		DomainRepo:  domainRepo,
		ProcessRepo: processRepo,
		RouteRepo:   routeRepo,
	}
}
