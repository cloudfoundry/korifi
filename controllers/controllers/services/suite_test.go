package services_test

import (
	"testing"
	"time"

	"code.cloudfoundry.org/korifi/controllers/fake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestServicesControllers(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Services Controllers Unit Test Suite")
}

var (
	fakeClient       *fake.Client
	fakeStatusWriter *fake.StatusWriter
)

var _ = BeforeEach(func() {
	fakeClient = new(fake.Client)
	fakeStatusWriter = &fake.StatusWriter{}
	fakeClient.StatusReturns(fakeStatusWriter)
})
