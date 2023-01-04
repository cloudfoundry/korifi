package errors_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApierrors(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Apierrors Suite")
}
