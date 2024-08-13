package params_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestInclude(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Include Suite")
}
