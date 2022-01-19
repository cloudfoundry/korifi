package apis

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"

	workloadsv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/workloads/v1alpha1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	k8sclient "k8s.io/client-go/kubernetes"

	"github.com/pivotal/kpack/pkg/logs"

	"code.cloudfoundry.org/cf-k8s-controllers/api/authorization"
	"github.com/go-logr/logr"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
	"github.com/gorilla/mux"
)

const (
	LogCacheInfoEndpoint = "/api/v1/info"
	LogCacheReadEndpoint = "/api/v1/read/{guid}"
	logCacheVersion      = "2.11.4+cf-k8s"
)

// LogCacheHandler implements the minimal set of log-cache API endpoints/features necessary
// to support the "cf push" workflow.
// It does not support actually fetching and returning application logs at this time
type LogCacheHandler struct {
	privilegedK8sClient k8sclient.Interface
	appRepo             CFAppRepository
	buildRepo           CFBuildRepository
	podRepo             PodRepository
	logger              logr.Logger
}

func NewLogCacheHandler(privilegedK8sClient k8sclient.Interface,
	appRepo CFAppRepository,
	buildRepo CFBuildRepository,
	podRepo PodRepository,
	logger logr.Logger) *LogCacheHandler {
	return &LogCacheHandler{
		privilegedK8sClient: privilegedK8sClient,
		appRepo:             appRepo,
		buildRepo:           buildRepo,
		podRepo:             podRepo,
		logger:              logger,
	}
}

func (h *LogCacheHandler) logCacheInfoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"version":"` + logCacheVersion + `","vm_uptime":"0"}`))
}

func (h *LogCacheHandler) logCacheEmptyReadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Since we're not currently returning an app logs there is no need to check the validity of the app guid
	// provided in the request. A full implementation of this endpoint needs to have the appropriate
	// validity and authorization checks in place.
	ctx := context.Background()
	vars := mux.Vars(r)
	appGUID := vars["guid"]

	app, err := h.appRepo.GetApp(ctx, authorization.Info{}, appGUID)
	if err != nil {
		switch err.(type) {
		case repositories.PermissionDeniedOrNotFoundError:
			h.logger.Info("App not found", "AppGUID", appGUID)
			writeNotFoundErrorResponse(w, "App")
			return
		default:
			h.logger.Error(err, "Failed to fetch app from Kubernetes", "AppGUID", appGUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	build, err := h.buildRepo.GetLatestBuildByAppGUID(ctx, authorization.Info{}, app.SpaceGUID, appGUID)
	if err != nil {
		switch err.(type) {
		case repositories.NotFoundError:
			h.logger.Info("Build not found", "BuildGUID", build.GUID)
			writeNotFoundErrorResponse(w, "Build")
			return
		default:
			h.logger.Error(err, "Failed to fetch build from Kubernetes", "BuildGUID", build.GUID)
			writeUnknownErrorResponse(w)
			return
		}
	}

	// Build logs
	logWriter := new(strings.Builder)
	err = logs.NewBuildLogsClient(h.privilegedK8sClient).GetImageLogs(ctx, logWriter, build.GUID, app.SpaceGUID)
	if err != nil {
		h.logger.Error(err, "Failed to fetch build logs from Kubernetes", "BuildGUID", build.GUID)
		writeUnknownErrorResponse(w)
		return
	}

	buildLogs := strings.Split(logWriter.String(), "\n")

	// App logs
	labelSelector, err := labels.ValidatedSelectorFromSet(map[string]string{
		workloadsv1alpha1.CFAppGUIDLabelKey:  app.GUID,
		"workloads.cloudfoundry.org/version": app.Revision,
	})
	if err != nil {
		h.logger.Error(err, "Failed to create label selector", "BuildGUID", build.GUID)
		writeUnknownErrorResponse(w)
		return
	}
	listOpts := client.ListOptions{Namespace: app.SpaceGUID, LabelSelector: labelSelector}

	pods, err := h.podRepo.ListPods(ctx, authorization.Info{}, listOpts)
	if err != nil {
		h.logger.Error(err, "Failed to fetch pods for app", "AppGUID", app.GUID)
		writeUnknownErrorResponse(w)
		return
	}

	applogs := []string{}

	for _, pod := range pods {
		logReadCloser, err := h.privilegedK8sClient.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).Stream(ctx)
		if err != nil {
			h.logger.Error(err, "Failed to fetch logs for pods", "podGUID", pod.Name)
			writeUnknownErrorResponse(w)
			return
		}

		defer logReadCloser.Close()
		r := bufio.NewReader(logReadCloser)
		for {
			line, err := r.ReadBytes('\n')
			if err != nil && err != io.EOF {
				writeUnknownErrorResponse(w)
				return
			}

			if err == io.EOF {
				break
			}

			applogs = append(applogs, string(line))
		}
	}

	logs := make(map[string][]string)
	logs["batch"] = append(buildLogs, applogs...)
	response := LogResponse{
		Envelopes: logs,
	}

	err = writeJsonResponse(w, response, http.StatusOK)
	if err != nil {
		h.logger.Error(err, "Failed to render response", "BuildGUID", build.GUID)
		writeUnknownErrorResponse(w)
	}
}

type LogResponse struct {
	Envelopes map[string][]string `json:"envelopes"`
}

func (h *LogCacheHandler) RegisterRoutes(router *mux.Router) {
	router.Path(LogCacheInfoEndpoint).Methods("GET").HandlerFunc(h.logCacheInfoHandler)
	router.Path(LogCacheReadEndpoint).Methods("GET").HandlerFunc(h.logCacheEmptyReadHandler)
}
