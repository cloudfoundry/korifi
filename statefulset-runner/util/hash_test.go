package util_test

import (
	"code.cloudfoundry.org/korifi/statefulset-runner/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Hash", func() {
	It("returns the truncated sha256 sum", func() {
		Expect(util.Hash("whatisthis")).To(Equal("8cd33337e2"))
	})
})
