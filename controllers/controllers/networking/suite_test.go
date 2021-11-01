package networking_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestWorkloadsControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Networking Controllers Unit Test Suite")
}
