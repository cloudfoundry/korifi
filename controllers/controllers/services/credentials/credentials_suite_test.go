package credentials_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCredentials(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Credentials Suite")
}
