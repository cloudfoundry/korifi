package filter_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFiltering(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Filtering Suite")
}
