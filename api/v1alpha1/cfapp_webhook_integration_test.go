// +build integration

package v1alpha1_test

import (
	. "github.com/onsi/gomega"
	"github.com/sclevine/spec"
	"testing"
)

var _ = AddToTestSuite("CFAppWebhook", testCFAppWebhook)

func testCFAppWebhook(t *testing.T, when spec.G, it spec.S) {
	g := NewWithT(t)
	g.Expect(true).To(BeTrue())
}
