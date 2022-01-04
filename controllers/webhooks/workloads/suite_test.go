package workloads_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestWorkloadsValidatingWebhooks(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Workloads Validating Webhooks Unit Test Suite")
}
