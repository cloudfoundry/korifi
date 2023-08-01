package middleware

import (
	"fmt"
	"net/http"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/routing"
	"github.com/Masterminds/semver"
	"github.com/go-logr/logr"
	"github.com/mileusna/useragent"
)

const minSupportedVersion = ">= 8.5.0"

func CFCliVersion(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logr.FromContextOrDiscard(r.Context()).WithName("cf-cli-version-check")
		userAgents := r.Header["User-Agent"]
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
			routing.PresentError(logger, w, apierrors.NewInvalidRequestError(
				err,
				fmt.Sprintf("Failed to determine CF CLI version. Korifi requires CF CLI %s", minSupportedVersion),
			))
			return
		}

		if !versionConstraint(logger).Check(cfCLIVersion) {
			routing.PresentError(logger, w, apierrors.NewInvalidRequestError(
				err,
				fmt.Sprintf("CF CLI version %s is not supported. Korifi requires CF CLI %s", cfCLIVersion, minSupportedVersion),
			))
			return

		}

		next.ServeHTTP(w, r)
	})
}

func versionConstraint(logger logr.Logger) *semver.Constraints {
	versionConstraint, err := semver.NewConstraint(minSupportedVersion)
	if err != nil {
		logger.Error(err, "failed to convert minSupportedVersion to version constraint", "minSupportedVersion", minSupportedVersion)
		return &semver.Constraints{}
	}
	return versionConstraint
}
