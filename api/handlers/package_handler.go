package handlers

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/payloads"
	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
)

const (
	PackagePath         = "/v3/packages/{guid}"
	PackagesPath        = "/v3/packages"
	PackageUploadPath   = "/v3/packages/{guid}/upload"
	PackageDropletsPath = "/v3/packages/{guid}/droplets"
)

//counterfeiter:generate -o fake -fake-name CFPackageRepository . CFPackageRepository
//counterfeiter:generate -o fake -fake-name ImageRepository . ImageRepository

type CFPackageRepository interface {
	GetPackage(context.Context, authorization.Info, string) (repositories.PackageRecord, error)
	ListPackages(context.Context, authorization.Info, repositories.ListPackagesMessage) ([]repositories.PackageRecord, error)
	CreatePackage(context.Context, authorization.Info, repositories.CreatePackageMessage) (repositories.PackageRecord, error)
	UpdatePackageSource(context.Context, authorization.Info, repositories.UpdatePackageSourceMessage) (repositories.PackageRecord, error)
}

type ImageRepository interface {
	UploadSourceImage(ctx context.Context, authInfo authorization.Info, imageRef string, srcReader io.Reader, spaceGUID string) (imageRefWithDigest string, err error)
}

type PackageHandler struct {
	handlerWrapper   *AuthAwareHandlerFuncWrapper
	serverURL        url.URL
	packageRepo      CFPackageRepository
	appRepo          CFAppRepository
	dropletRepo      CFDropletRepository
	imageRepo        ImageRepository
	decoderValidator *DecoderValidator
	registryBase     string
}

func NewPackageHandler(
	serverURL url.URL,
	packageRepo CFPackageRepository,
	appRepo CFAppRepository,
	dropletRepo CFDropletRepository,
	imageRepo ImageRepository,
	decoderValidator *DecoderValidator,
	registryBase string,
) *PackageHandler {
	return &PackageHandler{
		handlerWrapper:   NewAuthAwareHandlerFuncWrapper(ctrl.Log.WithName("PackageHandler")),
		serverURL:        serverURL,
		packageRepo:      packageRepo,
		appRepo:          appRepo,
		dropletRepo:      dropletRepo,
		imageRepo:        imageRepo,
		registryBase:     registryBase,
		decoderValidator: decoderValidator,
	}
}

func (h PackageHandler) packageGetHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	packageGUID := mux.Vars(r)["guid"]
	record, err := h.packageRepo.GetPackage(ctx, authInfo, packageGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error fetching package with repository")
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForPackage(record, h.serverURL)), nil
}

func (h PackageHandler) packageListHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	packageListQueryParameters := new(payloads.PackageListQueryParameters)
	err := payloads.Decode(packageListQueryParameters, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	records, err := h.packageRepo.ListPackages(ctx, authInfo, packageListQueryParameters.ToMessage())
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error fetching package with repository", "error")
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForPackageList(records, h.serverURL, *r.URL)), nil
}

func (h PackageHandler) packageCreateHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	var payload payloads.PackageCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "failed to decode payload")
	}

	appRecord, err := h.appRepo.GetApp(ctx, authInfo, payload.Relationships.App.Data.GUID)
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

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForPackage(record, h.serverURL)), nil
}

func (h PackageHandler) packageUploadHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	packageGUID := mux.Vars(r)["guid"]
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

	imageRef := path.Join(h.registryBase, packageGUID)
	uploadedImageRef, err := h.imageRepo.UploadSourceImage(r.Context(), authInfo, imageRef, bitsFile, record.SpaceGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error calling uploadSourceImage")
	}

	record, err = h.packageRepo.UpdatePackageSource(r.Context(), authInfo, repositories.UpdatePackageSourceMessage{
		GUID:      packageGUID,
		SpaceGUID: record.SpaceGUID,
		ImageRef:  uploadedImageRef,
	})
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error calling UpdatePackageSource")
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForPackage(record, h.serverURL)), nil
}

func (h PackageHandler) packageListDropletsHandler(ctx context.Context, logger logr.Logger, authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	if err := r.ParseForm(); err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to parse request query parameters")
	}

	packageListDropletsQueryParams := new(payloads.PackageListDropletsQueryParameters)
	err := payloads.Decode(packageListDropletsQueryParams, r.Form)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Unable to decode request query parameters")
	}

	packageGUID := mux.Vars(r)["guid"]
	_, err = h.packageRepo.GetPackage(r.Context(), authInfo, packageGUID)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, apierrors.ForbiddenAsNotFound(err), "Error fetching package with repository")
	}

	dropletListMessage := packageListDropletsQueryParams.ToMessage([]string{packageGUID})

	dropletList, err := h.dropletRepo.ListDroplets(r.Context(), authInfo, dropletListMessage)
	if err != nil {
		return nil, apierrors.LogAndReturn(logger, err, "Error fetching droplet list with repository")
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForDropletList(dropletList, h.serverURL, *r.URL)), nil
}

func (h *PackageHandler) RegisterRoutes(router *mux.Router) {
	router.Path(PackagePath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.packageGetHandler))
	router.Path(PackagesPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.packageListHandler))
	router.Path(PackagesPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.packageCreateHandler))
	router.Path(PackageUploadPath).Methods("POST").HandlerFunc(h.handlerWrapper.Wrap(h.packageUploadHandler))
	router.Path(PackageDropletsPath).Methods("GET").HandlerFunc(h.handlerWrapper.Wrap(h.packageListDropletsHandler))
}
