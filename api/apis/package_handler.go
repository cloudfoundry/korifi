package apis

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"

	"code.cloudfoundry.org/cf-k8s-controllers/api/apierrors"
	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"github.com/gorilla/schema"
)

const (
	PackageGetEndpoint          = "/v3/packages/{guid}"
	PackageListEndpoint         = "/v3/packages"
	PackageCreateEndpoint       = "/v3/packages"
	PackageUploadEndpoint       = "/v3/packages/{guid}/upload"
	PackageListDropletsEndpoint = "/v3/packages/{guid}/droplets"
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
	logger             logr.Logger
	serverURL          url.URL
	packageRepo        CFPackageRepository
	appRepo            CFAppRepository
	dropletRepo        CFDropletRepository
	imageRepo          ImageRepository
	decoderValidator   *DecoderValidator
	registryBase       string
	registrySecretName string
}

func NewPackageHandler(
	logger logr.Logger,
	serverURL url.URL,
	packageRepo CFPackageRepository,
	appRepo CFAppRepository,
	dropletRepo CFDropletRepository,
	imageRepo ImageRepository,
	decoderValidator *DecoderValidator,
	registryBase string,
	registrySecretName string,
) *PackageHandler {
	return &PackageHandler{
		logger:             logger,
		serverURL:          serverURL,
		packageRepo:        packageRepo,
		appRepo:            appRepo,
		dropletRepo:        dropletRepo,
		imageRepo:          imageRepo,
		registryBase:       registryBase,
		registrySecretName: registrySecretName,
		decoderValidator:   decoderValidator,
	}
}

func (h PackageHandler) packageGetHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	packageGUID := mux.Vars(r)["guid"]
	record, err := h.packageRepo.GetPackage(r.Context(), authInfo, packageGUID)
	if err != nil {
		switch {
		case errors.As(err, new(repositories.NotFoundError)):
			return nil, apierrors.NewNotFoundError(err, repositories.PackageResourceType)
		default:
			h.logger.Info("Error fetching package with repository", "error", err.Error())
			return nil, err
		}
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForPackage(record, h.serverURL)), nil
}

func (h PackageHandler) packageListHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		return nil, err
	}

	packageListQueryParameters := new(payloads.PackageListQueryParameters)
	err := schema.NewDecoder().Decode(packageListQueryParameters, r.Form)
	if err != nil {
		switch err.(type) {
		case schema.MultiError:
			multiError := err.(schema.MultiError)
			for _, v := range multiError {
				_, ok := v.(schema.UnknownKeyError)
				if ok {
					h.logger.Info("Unknown key used in Package filter")
					return nil, apierrors.NewUnknownKeyError(err, packageListQueryParameters.SupportedQueryParameters())
				}
			}
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		}
	}

	records, err := h.packageRepo.ListPackages(r.Context(), authInfo, packageListQueryParameters.ToMessage())
	if err != nil {
		h.logger.Error(err, "Error fetching package with repository", "error")
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForPackageList(records, h.serverURL, *r.URL)), nil
}

func (h PackageHandler) packageCreateHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	var payload payloads.PackageCreate
	if err := h.decoderValidator.DecodeAndValidateJSONPayload(r, &payload); err != nil {
		return nil, err
	}

	appRecord, err := h.appRepo.GetApp(r.Context(), authInfo, payload.Relationships.App.Data.GUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("App not found", "App GUID", payload.Relationships.App.Data.GUID)
			return nil, apierrors.NewUnprocessableEntityError(err, "App is invalid. Ensure it exists and you have access to it.")
		case repositories.ForbiddenError:
			h.logger.Info("App forbidden", "App GUID", payload.Relationships.App.Data.GUID)
			return nil, apierrors.NewUnprocessableEntityError(err, "App is invalid. Ensure it exists and you have access to it.")
		default:
			h.logger.Info("Error finding App", "App GUID", payload.Relationships.App.Data.GUID)
			return nil, err
		}
	}

	record, err := h.packageRepo.CreatePackage(r.Context(), authInfo, payload.ToMessage(appRecord))
	if err != nil {
		switch err.(type) {
		case repositories.ForbiddenError:
			h.logger.Info("Not authorized to create packages", "App Name", payload.Relationships.App, "error", err)
			return nil, apierrors.NewForbiddenError(err, repositories.PackageResourceType)
		default:
			h.logger.Info("Error creating package with repository", "error", err.Error())
			return nil, err
		}
	}

	return NewHandlerResponse(http.StatusCreated).WithBody(presenter.ForPackage(record, h.serverURL)), nil
}

func (h PackageHandler) packageUploadHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	packageGUID := mux.Vars(r)["guid"]
	err := r.ParseForm()
	if err != nil { // untested - couldn't find a way to trigger this branch
		h.logger.Info("Error parsing multipart form", "error", err.Error())
		return nil, apierrors.NewInvalidRequestError(err, "Unable to parse body as multipart form")
	}

	bitsFile, _, err := r.FormFile("bits")
	if err != nil {
		h.logger.Info("Error reading form file \"bits\"", "error", err.Error())
		return nil, apierrors.NewUnprocessableEntityError(err, "Upload must include bits")
	}
	defer bitsFile.Close()

	record, err := h.packageRepo.GetPackage(r.Context(), authInfo, packageGUID)
	if err != nil {
		switch {
		case errors.As(err, new(repositories.ForbiddenError)):
			h.logger.Info("Package forbidden", "Package GUID", packageGUID)
			return nil, apierrors.NewNotFoundError(err, repositories.PackageResourceType)
		case errors.As(err, new(repositories.NotFoundError)):
			return nil, apierrors.NewNotFoundError(err, repositories.PackageResourceType)
		default:
			h.logger.Info("Error fetching package with repository", "error", err.Error())
			return nil, err
		}
	}

	if record.State != repositories.PackageStateAwaitingUpload {
		h.logger.Info("Error, cannot call package upload state was not AWAITING_UPLOAD", "packageGUID", packageGUID)
		return nil, apierrors.NewPackageBitsAlreadyUploadedError(err)
	}

	imageRef := path.Join(h.registryBase, packageGUID)
	uploadedImageRef, err := h.imageRepo.UploadSourceImage(r.Context(), authInfo, imageRef, bitsFile, record.SpaceGUID)
	if err != nil {
		if errors.As(err, new(repositories.ForbiddenError)) {
			h.logger.Info("not authorized to upload source image", "error", err)
			return nil, apierrors.NewForbiddenError(err, repositories.SourceImageResourceType)
		}
		h.logger.Info("Error calling uploadSourceImage", "error", err.Error())
		return nil, err
	}

	record, err = h.packageRepo.UpdatePackageSource(r.Context(), authInfo, repositories.UpdatePackageSourceMessage{
		GUID:               packageGUID,
		SpaceGUID:          record.SpaceGUID,
		ImageRef:           uploadedImageRef,
		RegistrySecretName: h.registrySecretName,
	})
	if err != nil {
		if errors.As(err, new(repositories.ForbiddenError)) {
			h.logger.Info("Updating package is forbidden to the user", "Package GUID", packageGUID)
			return nil, apierrors.NewForbiddenError(err, repositories.PackageResourceType)
		}

		h.logger.Info("Error calling UpdatePackageSource", "error", err.Error())
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForPackage(record, h.serverURL)), nil
}

func (h PackageHandler) packageListDropletsHandler(authInfo authorization.Info, r *http.Request) (*HandlerResponse, error) {
	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		return nil, err
	}

	packageListDropletsQueryParams := new(payloads.PackageListDropletsQueryParameters)
	err := schema.NewDecoder().Decode(packageListDropletsQueryParams, r.Form)
	if err != nil {
		switch err.(type) {
		case schema.MultiError:
			multiError := err.(schema.MultiError)
			for _, v := range multiError {
				_, ok := v.(schema.UnknownKeyError)
				if ok {
					h.logger.Info("Unknown key used in Package filter")
					return nil, apierrors.NewUnknownKeyError(err, packageListDropletsQueryParams.SupportedQueryParameters())
				}
			}
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			return nil, err
		}
	}

	packageGUID := mux.Vars(r)["guid"]
	_, err = h.packageRepo.GetPackage(r.Context(), authInfo, packageGUID)
	if err != nil {
		switch {
		case errors.As(err, new(repositories.NotFoundError)):
			return nil, apierrors.NewNotFoundError(err, repositories.PackageResourceType)
		default:
			h.logger.Info("Error fetching package with repository", "error", err.Error())
			return nil, err
		}
	}

	dropletListMessage := packageListDropletsQueryParams.ToMessage([]string{packageGUID})

	dropletList, err := h.dropletRepo.ListDroplets(r.Context(), authInfo, dropletListMessage)
	if err != nil {
		h.logger.Info("Error fetching droplet list with repository", "error", err.Error())
		return nil, err
	}

	return NewHandlerResponse(http.StatusOK).WithBody(presenter.ForDropletList(dropletList, h.serverURL, *r.URL)), nil
}

func (h *PackageHandler) RegisterRoutes(router *mux.Router) {
	w := NewAuthAwareHandlerFuncWrapper(h.logger)
	router.Path(PackageGetEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.packageGetHandler))
	router.Path(PackageListEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.packageListHandler))
	router.Path(PackageCreateEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.packageCreateHandler))
	router.Path(PackageUploadEndpoint).Methods("POST").HandlerFunc(w.Wrap(h.packageUploadHandler))
	router.Path(PackageListDropletsEndpoint).Methods("GET").HandlerFunc(w.Wrap(h.packageListDropletsHandler))
}
