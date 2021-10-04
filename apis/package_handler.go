package apis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"

	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"

	"code.cloudfoundry.org/cf-k8s-api/payloads"

	"code.cloudfoundry.org/cf-k8s-api/presenter"

	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"code.cloudfoundry.org/cf-k8s-api/repositories"
)

const (
	PackageCreateEndpoint = "/v3/packages"
	PackageUploadEndpoint = "/v3/packages/{guid}/upload"
)

//counterfeiter:generate -o fake -fake-name CFPackageRepository . CFPackageRepository

type CFPackageRepository interface {
	FetchPackage(context.Context, client.Client, string) (repositories.PackageRecord, error)
	CreatePackage(context.Context, client.Client, repositories.PackageCreateMessage) (repositories.PackageRecord, error)
	UpdatePackageSource(ctx context.Context, client client.Client, message repositories.PackageUpdateSourceMessage) (repositories.PackageRecord, error)
}

//counterfeiter:generate -o fake -fake-name SourceImageUploader . SourceImageUploader

type SourceImageUploader func(imageRef string, packageSrcFile multipart.File, credentialOption remote.Option) (imageRefWithDigest string, err error)

//counterfeiter:generate -o fake -fake-name RegistryAuthBuilder . RegistryAuthBuilder

type RegistryAuthBuilder func(ctx context.Context) (remote.Option, error)

type PackageHandler struct {
	logger             logr.Logger
	serverURL          string
	packageRepo        CFPackageRepository
	appRepo            CFAppRepository
	buildClient        ClientBuilder
	uploadSourceImage  SourceImageUploader
	buildRegistryAuth  RegistryAuthBuilder
	k8sConfig          *rest.Config
	registryBase       string
	registrySecretName string
}

func NewPackageHandler(logger logr.Logger, serverURL string, packageRepo CFPackageRepository, appRepo CFAppRepository, buildClient ClientBuilder, uploadSourceImage SourceImageUploader, buildRegistryAuth RegistryAuthBuilder, k8sConfig *rest.Config, registryBase string, registrySecretName string) *PackageHandler {
	return &PackageHandler{
		logger:             logger,
		serverURL:          serverURL,
		packageRepo:        packageRepo,
		appRepo:            appRepo,
		buildClient:        buildClient,
		uploadSourceImage:  uploadSourceImage,
		buildRegistryAuth:  buildRegistryAuth,
		k8sConfig:          k8sConfig,
		registryBase:       registryBase,
		registrySecretName: registrySecretName,
	}
}

func (h PackageHandler) packageCreateHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var payload payloads.PackageCreate
	rme := DecodeAndValidatePayload(req, &payload)
	if rme != nil {
		writeErrorResponse(w, rme)
		return
	}

	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Info("Error building k8s client", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	appRecord, err := h.appRepo.FetchApp(req.Context(), client, payload.Relationships.App.Data.GUID)
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

	record, err := h.packageRepo.CreatePackage(req.Context(), client, payload.ToMessage(appRecord.SpaceGUID))
	if err != nil {
		h.logger.Info("Error creating package with repository", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	res := presenter.ForPackage(record, h.serverURL)
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(res)
	if err != nil { // untested
		h.logger.Info("Error encoding JSON response", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}
}

func (h PackageHandler) packageUploadHandler(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	packageGUID := mux.Vars(req)["guid"]
	err := req.ParseForm()
	if err != nil { // untested - couldn't find a way to trigger this branch
		h.logger.Info("Error parsing multipart form", "error", err.Error())
		writeInvalidRequestError(w, "Unable to parse body as multipart form")
		return
	}

	bitsFile, _, err := req.FormFile("bits")
	if err != nil {
		h.logger.Info("Error reading form file \"bits\"", "error", err.Error())
		writeUnprocessableEntityError(w, "Upload must include bits")
		return
	}
	defer bitsFile.Close()

	client, err := h.buildClient(h.k8sConfig)
	if err != nil {
		h.logger.Info("Error building k8s client", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	record, err := h.packageRepo.FetchPackage(req.Context(), client, packageGUID)
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

	registryAuth, err := h.buildRegistryAuth(req.Context())
	if err != nil {
		h.logger.Info("Error calling buildRegistryAuth", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	imageRef := fmt.Sprintf("%s/%s", h.registryBase, packageGUID)

	uploadedImageRef, err := h.uploadSourceImage(imageRef, bitsFile, registryAuth)
	if err != nil {
		h.logger.Info("Error calling uploadSourceImage", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}

	record, err = h.packageRepo.UpdatePackageSource(req.Context(), client, repositories.PackageUpdateSourceMessage{
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

	res := presenter.ForPackage(record, h.serverURL)
	err = json.NewEncoder(w).Encode(res)
	if err != nil { // untested
		h.logger.Info("Error encoding JSON response", "error", err.Error())
		writeUnknownErrorResponse(w)
		return
	}
}

func (h *PackageHandler) RegisterRoutes(router *mux.Router) {
	router.Path(PackageCreateEndpoint).Methods("POST").HandlerFunc(h.packageCreateHandler)
	router.Path(PackageUploadEndpoint).Methods("POST").HandlerFunc(h.packageUploadHandler)
}
