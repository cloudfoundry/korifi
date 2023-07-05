package coordination_test

import (
	"code.cloudfoundry.org/korifi/controllers/coordination"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("HashName", func() {
	It("hashes name by entity type", func() {
		Expect(coordination.HashName("foo", "bar")).To(Equal("n-1ae6b1a6a110c82ea8dc01dc427544178c0ee365"))
	})
})
