package dockerutils_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDockerutils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dockerutils Suite")
}
