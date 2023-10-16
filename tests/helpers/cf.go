package helpers

import (
	"github.com/cloudfoundry/cf-test-helpers/cf"
	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	"github.com/onsi/gomega/gexec"
)

func Cf(args ...string) *gexec.Session {
	GinkgoHelper()

	return cf.Cf(args...).Wait()
}
