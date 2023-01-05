package handlers

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/authorization"
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/routing"

	"github.com/go-logr/logr"
)

const (
	PackagePath         = "/v3/packages/{guid}"
	PackagesPath        = "/v3/packages"
	PackageUploadPath   = "/v3/packages/{guid}/upload"
	PackageDropletsPath = "/v3/packages/{guid}/droplets"
)

//counterfeiter:generate -o fake -fake-name CFPackageRepository . CFPackageRepository
//counterfeiter:generate -o fake -fake-name ImageRepository . ImageRepository
//counterfeiter:generate -o fake -fake-name RequestJSONValidator . RequestJSONValidator

type CFPackageRepository interface {
	GetPackage(context.Context, authorization.Info, string) (repositories.PackageRecord, error)
	ListPackages(context.Context, authorization.Info, repositories.ListPackagesMessage) ([]repositories.PackageRecord, error)
	CreatePackage(context.Context, authorization.Info, repositories.CreatePackageMessage) (repositories.PackageRecord, error)
	UpdatePackageSource(context.Context, authorization.Info, repositories.UpdatePackageSourceMessage) (repositories.PackageRecord, error)
	UpdatePackage(context.Context, authorization.Info, repositories.UpdatePackageMessage) (repositories.PackageRecord, error)
}

type ImageRepository interface {
	UploadSourceImage(ctx context.Context, authInfo authorization.Info, imageRef string, srcReader io.Reader, spaceGUID string) (imageRefWithDigest string, err error)
}

type RequestJSONValidator interface {
	DecodeAndValidateJSONPayload(r *http.Request, object interface{}) error
}

type Package struct {
	serverURL          url.URL
	packageRepo        CFPackageRepository
	appRepo            CFAppRepository
	dropletRepo        CFDropletRepository
	imageRepo          ImageRepository
	requestValidator   RequestJSONValidator
	registrySecretName string
}

func NewPackage(
	serverURL url.URL,
	packageRepo CFPackageRepository,
	appRepo CFAppRepository,
	dropletRepo CFDropletRepository,
	imageRepo ImageRepository,
	requestValidator RequestJSONValidator,
	registrySecretName string,
) *Package {
	return &Package{
		serverURL:          serverURL,
		packageRepo:        packageRepo,
		appRepo:            appRepo,
		dropletRepo:        dropletRepo,
		imageRepo:          imageRepo,
		registrySecretName: registrySecretName,
		requestValidator:   requestValidator,
	}
}

func (h Package) get(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.package.get")

	packageGUID := routing.URLParam(r, "guid")
	record, err := h.packageRepo.GetPackage(r.Context(), authInfo, packageGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error fetching package with repository")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForPackage(record, h.serverURL)), nil
}

func (h Package) list(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.package.list")

	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	packageListQueryParameters := new(payloads.PackageListQueryParameters)
	err := payloads.Decode(packageListQueryParameters, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	records, err := h.packageRepo.ListPackages(r.Context(), authInfo, packageListQueryParameters.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error fetching package with repository", "error")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForPackageList(records, h.serverURL, *r.URL)), nil
}

func (h Package) create(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.package.create")

	var payload payloads.PackageCreate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	appRecord, err := h.appRepo.GetApp(r.Context(), authInfo, payload.Relationships.App.Data.GUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(
			logger,
			apierrors.AsUnprocessableEntity(
				err,
				"App is invalid. Ensure it exists and you have access to it.",
				apierrors.NotFoundError{},
				apierrors.ForbiddenError{},
			),
			"Error finding App",
			"App GUID", payload.Relationships.App.Data.GUID,
		)
	}

	record, err := h.packageRepo.CreatePackage(r.Context(), authInfo, payload.ToMessage(appRecord))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error creating package with repository")
	}

	return routing.NewResponse(http.StatusCreated).WithBody(presenter.ForPackage(record, h.serverURL)), nil
}

func (h Package) update(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.package.update")

	var payload payloads.PackageUpdate
	if err := h.requestValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	packageGUID := routing.URLParam(r, "guid")
	packageRecord, err := h.packageRepo.UpdatePackage(r.Context(), authInfo, payload.ToMessage(packageGUID))
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error updating package")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForPackage(packageRecord, h.serverURL)), nil
}

func (h Package) upload(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.package.upload")

	packageGUID := routing.URLParam(r, "guid")
	err := r.ParseForm()
	if err != nil { // untested - couldn't find a way to trigger this branch
		return nil, apierrors.LogAndReturn(logger, apierrors.NewInvalidRequestError(err, "Unable to parse body as multipart form"), "Error parsing multipart form")
	}

	bitsFile, _, err := r.FormFile("bits")
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.NewUnprocessableEntityError(err, "Upload must include bits"), "Error reading form file \"bits\"")
	}
	defer bitsFile.Close()

	record, err := h.packageRepo.GetPackage(r.Context(), authInfo, packageGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error fetching package with repository")
	}

	if record.State != repositories.PackageStateAwaitingUpload {
		return nil, apierrors.LogAndReturn(logger, apierrors.NewPackageBitsAlreadyUploadedError(err), "Error, cannot call package upload state was not AWAITING_UPLOAD", "packageGUID", packageGUID)
	}

	uploadedImageRef, err := h.imageRepo.UploadSourceImage(r.Context(), authInfo, record.ImageRef, bitsFile, record.SpaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error calling uploadSourceImage")
	}

	record, err = h.packageRepo.UpdatePackageSource(r.Context(), authInfo, repositories.UpdatePackageSourceMessage{
		GUID:               packageGUID,
		SpaceGUID:          record.SpaceGUID,
		ImageRef:           uploadedImageRef,
		RegistrySecretName: h.registrySecretName,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error calling UpdatePackageSource")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForPackage(record, h.serverURL)), nil
}

func (h Package) listDroplets(r *http.Request) (*routing.Response, error) {
	authInfo, _ := authorization.InfoFromContext(r.Context())
	logger := logr.FromContextOrDiscard(r.Context()).WithName("handlers.package.list-droplets")

	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	packageListDropletsQueryParams := new(payloads.PackageListDropletsQueryParameters)
	err := payloads.Decode(packageListDropletsQueryParams, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	packageGUID := routing.URLParam(r, "guid")
	_, err = h.packageRepo.GetPackage(r.Context(), authInfo, packageGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error fetching package with repository")
	}

	dropletListMessage := packageListDropletsQueryParams.ToMessage([]string{packageGUID})

	dropletList, err := h.dropletRepo.ListDroplets(r.Context(), authInfo, dropletListMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error fetching droplet list with repository")
	}

	return routing.NewResponse(http.StatusOK).WithBody(presenter.ForDropletList(dropletList, h.serverURL, *r.URL)), nil
}

func (h *Package) UnauthenticatedRoutes() []routing.Route {
	return nil
}

func (h *Package) AuthenticatedRoutes() []routing.Route {
	return []routing.Route{
		{Method: "GET", Pattern: PackagePath, Handler: h.get},
		{Method: "PATCH", Pattern: PackagePath, Handler: h.update},
		{Method: "GET", Pattern: PackagesPath, Handler: h.list},
		{Method: "POST", Pattern: PackagesPath, Handler: h.create},
		{Method: "POST", Pattern: PackageUploadPath, Handler: h.upload},
		{Method: "GET", Pattern: PackageDropletsPath, Handler: h.listDroplets},
	}
}
