package ginkgo_parallel_count_test

import (
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGinkgoParallelCount(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GinkgoParallelCount Suite")
}

var _ = Describe("Parallel Counter", func() {
	It("outputs the number of parallel nodes", func() {
		cfg, _ := GinkgoConfiguration()
		fmt.Printf("ParallelNodes: %d\n", cfg.ParallelTotal)
	})
})
