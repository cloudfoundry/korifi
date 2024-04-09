package helpers

import (
	"github.com/cloudfoundry/cf-test-helpers/cf"
	. "github.com/onsi/ginkgo/v2"    //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega/gexec" //lint:ignore ST1001 this is a test file
)

func Cf(args ...string) *Session {
	GinkgoHelper()

	return cf.Cf(args...).Wait()
}

func GetApiServerRoot() string {
	return GetRequiredEnvVar("API_SERVER_ROOT")
}
