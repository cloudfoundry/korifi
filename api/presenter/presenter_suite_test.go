package presenter_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPresenter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Presenter Suite")
}
