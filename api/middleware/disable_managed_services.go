package middleware

import (
	"net/http"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/routing"
	"github.com/go-logr/logr"
)

func DisableManagedServices(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logr.FromContextOrDiscard(r.Context()).WithName("disable-managed-services")

		if isManagedServicesEndpoint(r.URL.Path) {
			routing.PresentError(logger, w, apierrors.NewInvalidRequestError(nil, "Experimental managed services support is not enabled"))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func isManagedServicesEndpoint(requestPath string) bool {
	if strings.HasPrefix(requestPath, "/v3/service_brokers") {
		return true
	}

	if strings.HasPrefix(requestPath, "/v3/service_offerings") {
		return true
	}

	if strings.HasPrefix(requestPath, "/v3/service_plans") {
		return true
	}

	return false
}
