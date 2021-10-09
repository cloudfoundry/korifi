package apis

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"code.cloudfoundry.org/cf-k8s-api/presenter"
	"code.cloudfoundry.org/cf-k8s-api/repositories"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

const (
	SpaceListEndpoint = "/v3/spaces"
)

//counterfeiter:generate -o fake -fake-name CFSpaceRepository . CFSpaceRepository

type CFSpaceRepository interface {
	FetchSpaces(context.Context, []string, []string) ([]repositories.SpaceRecord, error)
}

type SpaceHandler struct {
	spaceRepo  CFSpaceRepository
	logger     logr.Logger
	apiBaseURL url.URL
}

func NewSpaceHandler(spaceRepo CFSpaceRepository, apiBaseURL url.URL) *SpaceHandler {
	return &SpaceHandler{
		spaceRepo:  spaceRepo,
		apiBaseURL: apiBaseURL,
		logger:     controllerruntime.Log.WithName("Org Handler"),
	}
}

func (h *SpaceHandler) SpaceListHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	w.Header().Set("Content-Type", "application/json")

	orgUIDs := parseCommaSeparatedList(r.URL.Query().Get("organization_guids"))
	names := parseCommaSeparatedList(r.URL.Query().Get("names"))

	spaces, err := h.spaceRepo.FetchSpaces(ctx, orgUIDs, names)
	if err != nil {
		writeUnknownErrorResponse(w)

		return
	}

	spaceList := presenter.ForSpaceList(spaces, h.apiBaseURL)
	json.NewEncoder(w).Encode(spaceList)
}

func (h *SpaceHandler) RegisterRoutes(router *mux.Router) {
	router.Path(SpaceListEndpoint).Methods("GET").HandlerFunc(h.SpaceListHandler)
}

func parseCommaSeparatedList(list string) []string {
	var elements []string
	for _, element := range strings.Split(list, ",") {
		if element != "" {
			elements = append(elements, element)
		}
	}

	return elements
}
