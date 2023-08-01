package version

import (
	"github.com/Masterminds/semver/v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const KorifiCreationVersionKey = "korifi.cloudfoundry.org/creation-version"

// version is overwritten at compile time by passing
// -ldflags -X code.cloudfoundry.org/korifi/version.Version=<version>
var Version = "v9999.99.99-local.dev"

type Checker struct {
	version *semver.Version
}

func NewChecker(ver string) Checker {
	return Checker{version: semver.MustParse(ver)}
}

func (c Checker) ObjectIsNewer(obj client.Object) (bool, error) {
	korifiVersion := obj.GetAnnotations()[KorifiCreationVersionKey]
	semVersion, err := semver.NewVersion(korifiVersion)
	if err != nil {
		return false, err
	}

	return semVersion.GreaterThan(c.version), nil
}
