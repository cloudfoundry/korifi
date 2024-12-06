package sbio_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSbio(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sbio Suite")
}
