package coordination_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestCoordination(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Coordination Suite")
}
