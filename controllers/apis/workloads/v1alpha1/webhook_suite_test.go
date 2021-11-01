package v1alpha1_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestWorkloadsMutatingWebhooks(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workloads Mutating Webhooks Unit Test Suite")
}
