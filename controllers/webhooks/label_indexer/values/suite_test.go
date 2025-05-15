package values_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestValues(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Index Values Suite")
}
