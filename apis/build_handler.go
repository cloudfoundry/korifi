package apis

import (
	"context"
	"encoding/json"
	"net/http"

	"code.cloudfoundry.org/cf-k8s-api/presenter"
	"code.cloudfoundry.org/cf-k8s-api/repositories"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	BuildGetEndpoint = "/v3/builds/{guid}"
)

//counterfeiter:generate -o fake -fake-name CFBuildRepository . CFBuildRepository
type CFBuildRepository interface {
	FetchBuild(context.Context, client.Client, string) (repositories.BuildRecord, error)
}

type BuildHandler struct {
	ServerURL   string
	BuildRepo   CFBuildRepository
	BuildClient ClientBuilder
	Logger      logr.Logger
	K8sConfig   *rest.Config
}

func (h *BuildHandler) BuildGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	vars := mux.Vars(r)
	buildGUID := vars["guid"]

	// TODO: Instantiate config based on bearer token
	// Spike code from EMEA folks around this: https://github.com/cloudfoundry/cf-crd-explorations/blob/136417fbff507eb13c92cd67e6fed6b061071941/cfshim/handlers/app_handler.go#L78
	client, err := h.BuildClient(h.K8sConfig)
	if err != nil {
		h.Logger.Error(err, "Unable to create Kubernetes client", "BuildGUID", buildGUID)
		writeUnknownErrorResponse(w)
		return
	}

	build, err := h.BuildRepo.FetchBuild(ctx, client, buildGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.Logger.Info("Build not found", "BuildGUID", buildGUID)
			writeNotFoundErrorResponse(w, "Build")
			return
		default:
			h.Logger.Error(err, "Failed to fetch build from Kubernetes", "BuildGUID", buildGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	responseBody, err := json.Marshal(presenter.ForBuild(build, h.ServerURL))
	if err != nil {
		h.Logger.Error(err, "Failed to render response", "BuildGUID", buildGUID)
		writeUnknownErrorResponse(w)
		return
	}
	w.Write(responseBody)
}

func (h *BuildHandler) RegisterRoutes(router *mux.Router) {
	router.Path(BuildGetEndpoint).Methods("GET").HandlerFunc(h.BuildGetHandler)

}
