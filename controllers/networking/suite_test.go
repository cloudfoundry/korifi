package networking

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestNetworkingControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Networking Controllers Unit Test Suite")
}
