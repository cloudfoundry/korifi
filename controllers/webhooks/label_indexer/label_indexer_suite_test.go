package label_indexer_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestLabelIndexer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "LabelIndexer Suite")
}
