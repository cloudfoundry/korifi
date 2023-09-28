package dockercfg_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDockercfg(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dockercfg Suite")
}
