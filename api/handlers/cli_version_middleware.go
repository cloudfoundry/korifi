package handlers

import (
	"fmt"
	"net/http"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/correlation"
	"github.com/Masterminds/semver"
	"github.com/go-http-utils/headers"
	"github.com/go-logr/logr"
	"github.com/mileusna/useragent"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const minSupportedVersion = ">= 8.5.0"

type CFCliVersionMiddleware struct{}

func NewCFCliVersionMiddleware() *CFCliVersionMiddleware {
	return &CFCliVersionMiddleware{}
}

func (m CFCliVersionMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := correlation.AddCorrelationIDToLogger(r.Context(), logf.Log.WithName("cf-cli-version-check"))
		userAgents := r.Header[headers.UserAgent]
		if len(userAgents) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		userAgent := useragent.Parse(userAgents[0])
		if userAgent.Name != "cf" {
			next.ServeHTTP(w, r)
			return
		}

		cfCLIVersion, err := semver.NewVersion(userAgent.Version)
		if err != nil {
			presentError(logger, w, apierrors.NewInvalidRequestError(
				err,
				fmt.Sprintf("Failed to determine CF CLI version. Korifi requires CF CLI %s", minSupportedVersion),
			))
			return
		}

		if !m.versionConstraint(logger).Check(cfCLIVersion) {
			presentError(logger, w, apierrors.NewInvalidRequestError(
				err,
				fmt.Sprintf("CF CLI version %s is not supported. Korifi requires CF CLI %s", cfCLIVersion, minSupportedVersion),
			))
			return

		}

		next.ServeHTTP(w, r)
	})
}

func (m CFCliVersionMiddleware) versionConstraint(logger logr.Logger) *semver.Constraints {
	versionConstraint, err := semver.NewConstraint(minSupportedVersion)
	if err != nil {
		logger.Error(err, "failed to convert minSupportedVersion to version constraint", "minSupportedVersion", minSupportedVersion)
		return &semver.Constraints{}
	}
	return versionConstraint
}
