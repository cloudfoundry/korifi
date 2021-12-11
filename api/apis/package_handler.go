package apis

import (
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/presenter"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/v1/remote"
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

type CFPackageRepository interface {
	FetchPackage(context.Context, authorization.Info, string) (repositories.PackageRecord, error)
	FetchPackageList(context.Context, authorization.Info, repositories.PackageListMessage) ([]repositories.PackageRecord, error)
	CreatePackage(context.Context, authorization.Info, repositories.PackageCreateMessage) (repositories.PackageRecord, error)
	UpdatePackageSource(context.Context, authorization.Info, repositories.PackageUpdateSourceMessage) (repositories.PackageRecord, error)
}

//counterfeiter:generate -o fake -fake-name SourceImageUploader . SourceImageUploader

type SourceImageUploader func(imageRef string, packageSrcFile multipart.File, credentialOption remote.Option) (imageRefWithDigest string, err error)

//counterfeiter:generate -o fake -fake-name RegistryAuthBuilder . RegistryAuthBuilder

type RegistryAuthBuilder func(ctx context.Context) (remote.Option, error)

type PackageHandler struct {
	logger             logr.Logger
	serverURL          url.URL
	packageRepo        CFPackageRepository
	appRepo            CFAppRepository
	dropletRepo        CFDropletRepository
	uploadSourceImage  SourceImageUploader
	buildRegistryAuth  RegistryAuthBuilder
	registryBase       string
	registrySecretName string
}

func NewPackageHandler(
	logger logr.Logger,
	serverURL url.URL,
	packageRepo CFPackageRepository,
	appRepo CFAppRepository,
	dropletRepo CFDropletRepository,
	uploadSourceImage SourceImageUploader,
	buildRegistryAuth RegistryAuthBuilder,
	registryBase string,
	registrySecretName string) *PackageHandler {
	return &PackageHandler{
		logger:             logger,
		serverURL:          serverURL,
		packageRepo:        packageRepo,
		appRepo:            appRepo,
		dropletRepo:        dropletRepo,
		uploadSourceImage:  uploadSourceImage,
		buildRegistryAuth:  buildRegistryAuth,
		registryBase:       registryBase,
		registrySecretName: registrySecretName,
	}
}

func (h PackageHandler) packageGetHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	packageGUID := mux.Vars(r)["guid"]
	record, err := h.packageRepo.FetchPackage(r.Context(), authInfo, packageGUID)
	if err != nil {
		switch {
		case errors.As(err, new(repositories.NotFoundError)):
			writeNotFoundErrorResponse(w, "Package")
		default:
			h.logger.Info("Error fetching package with repository", "error", err.Error())
			writeUnknownErrorResponse(w)
		}
		return
	}

	err = writeJsonResponse(w, presenter.ForPackage(record, h.serverURL), http.StatusOK)
	if err != nil {
		h.logger.Info("Error encoding JSON response", "error", err.Error())
		writeUnknownErrorResponse(w)
	}
}

func (h PackageHandler) packageListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		writeUnknownErrorResponse(w)
		return
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
					writeUnknownKeyError(w, packageListQueryParameters.SupportedQueryParameters())
					return
				}
			}
			h.logger.Error(err, "Unable to decode request query parameters")
			writeUnknownErrorResponse(w)
			return

		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			writeUnknownErrorResponse(w)
			return
		}
	}

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	records, err := h.packageRepo.FetchPackageList(r.Context(), authInfo, packageListQueryParameters.ToMessage())
	if err != nil {
		h.logger.Error(err, "Error fetching package with repository", "error")
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForPackageList(records, h.serverURL, *r.URL), http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Error encoding JSON response", "error")
		writeUnknownErrorResponse(w)
	}
}

func (h PackageHandler) packageCreateHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var payload payloads.PackageCreate
	rme := decodeAndValidateJSONPayload(r, &payload)
	if rme != nil {
		writeRequestMalformedErrorResponse(w, rme)
		return
	}

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	appRecord, err := h.appRepo.FetchApp(r.Context(), authInfo, payload.Relationships.App.Data.GUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("App not found", "App GUID", payload.Relationships.App.Data.GUID)
			writeUnprocessableEntityError(w, "App is invalid. Ensure it exists and you have access to it.")
		default:
			h.logger.Info("Error finding App", "App GUID", payload.Relationships.App.Data.GUID)
			writeUnknownErrorResponse(w)
		}
		return
	}

	record, err := h.packageRepo.CreatePackage(r.Context(), authInfo, payload.ToMessage(appRecord.SpaceGUID))
	if err != nil {
		h.logger.Info("Error creating package with repository", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForPackage(record, h.serverURL), http.StatusCreated)
	if err != nil { // untested
		h.logger.Info("Error encoding JSON response", "error", err.Error())
		writeUnknownErrorResponse(w)
	}
}

func (h PackageHandler) packageUploadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	packageGUID := mux.Vars(r)["guid"]
	err := r.ParseForm()
	if err != nil { // untested - couldn't find a way to trigger this branch
		h.logger.Info("Error parsing multipart form", "error", err.Error())
		writeInvalidRequestError(w, "Unable to parse body as multipart form")
		return
	}

	bitsFile, _, err := r.FormFile("bits")
	if err != nil {
		h.logger.Info("Error reading form file \"bits\"", "error", err.Error())
		writeUnprocessableEntityError(w, "Upload must include bits")
		return
	}
	defer bitsFile.Close()

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	record, err := h.packageRepo.FetchPackage(r.Context(), authInfo, packageGUID)
	if err != nil {
		switch {
		case errors.As(err, new(repositories.NotFoundError)):
			writeNotFoundErrorResponse(w, "Package")
		default:
			h.logger.Info("Error fetching package with repository", "error", err.Error())
			writeUnknownErrorResponse(w)
		}
		return
	}

	if record.State != repositories.PackageStateAwaitingUpload {
		h.logger.Info("Error, cannot call package upload state was not AWAITING_UPLOAD", "packageGUID", packageGUID)
		writePackageBitsAlreadyUploadedError(w)
		return
	}

	registryAuth, err := h.buildRegistryAuth(r.Context())
	if err != nil {
		h.logger.Info("Error calling buildRegistryAuth", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	imageRef := path.Join(h.registryBase, packageGUID)

	uploadedImageRef, err := h.uploadSourceImage(imageRef, bitsFile, registryAuth)
	if err != nil {
		h.logger.Info("Error calling uploadSourceImage", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	record, err = h.packageRepo.UpdatePackageSource(r.Context(), authInfo, repositories.PackageUpdateSourceMessage{
		GUID:               packageGUID,
		SpaceGUID:          record.SpaceGUID,
		ImageRef:           uploadedImageRef,
		RegistrySecretName: h.registrySecretName,
	})
	if err != nil {
		h.logger.Info("Error calling UpdatePackageSource", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForPackage(record, h.serverURL), http.StatusOK)
	if err != nil { // untested
		h.logger.Info("Error encoding JSON response", "error", err.Error())
		writeUnknownErrorResponse(w)
	}
}

func (h PackageHandler) packageListDropletsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseForm(); err != nil {
		h.logger.Error(err, "Unable to parse request query parameters")
		writeUnknownErrorResponse(w)
		return
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
					writeUnknownKeyError(w, packageListDropletsQueryParams.SupportedQueryParameters())
					return
				}
			}
			h.logger.Error(err, "Unable to decode request query parameters")
			writeUnknownErrorResponse(w)
			return

		default:
			h.logger.Error(err, "Unable to decode request query parameters")
			writeUnknownErrorResponse(w)
			return
		}
	}

	authInfo, ok := authorization.InfoFromContext(r.Context())
	if !ok {
		h.logger.Error(nil, "unable to get auth info")
		writeUnknownErrorResponse(w)
		return
	}

	packageGUID := mux.Vars(r)["guid"]
	_, err = h.packageRepo.FetchPackage(r.Context(), authInfo, packageGUID)
	if err != nil {
		switch {
		case errors.As(err, new(repositories.NotFoundError)):
			writeNotFoundErrorResponse(w, "Package")
		default:
			h.logger.Info("Error fetching package with repository", "error", err.Error())
			writeUnknownErrorResponse(w)
		}
		return
	}

	dropletListMessage := packageListDropletsQueryParams.ToMessage([]string{packageGUID})

	dropletList, err := h.dropletRepo.FetchDropletList(r.Context(), authInfo, dropletListMessage)
	if err != nil {
		h.logger.Info("Error fetching droplet list with repository", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	err = writeJsonResponse(w, presenter.ForDropletList(dropletList, h.serverURL, *r.URL), http.StatusOK)
	if err != nil { // Untested
		h.logger.Error(err, "Failed to render response")
		writeUnknownErrorResponse(w)
	}
}

func (h *PackageHandler) RegisterRoutes(router *mux.Router) {
	router.Path(PackageGetEndpoint).Methods("GET").HandlerFunc(h.packageGetHandler)
	router.Path(PackageListEndpoint).Methods("GET").HandlerFunc(h.packageListHandler)
	router.Path(PackageCreateEndpoint).Methods("POST").HandlerFunc(h.packageCreateHandler)
	router.Path(PackageUploadEndpoint).Methods("POST").HandlerFunc(h.packageUploadHandler)
	router.Path(PackageListDropletsEndpoint).Methods("GET").HandlerFunc(h.packageListDropletsHandler)
}
