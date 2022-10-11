package controllers_test

import (
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/fake"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNetworkingControllers(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Contour Router Unit Test Suite")
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

func expectCFRouteValidStatus(cfRouteStatus korifiv1alpha1.CFRouteStatus, valid bool, desc ...string) {
	validCondition := meta.FindStatusCondition(cfRouteStatus.Conditions, "Valid")
	Expect(validCondition).NotTo(BeNil())

	if valid {
		Expect(cfRouteStatus.FQDN).ToNot(BeEmpty())
		Expect(cfRouteStatus.URI).ToNot(BeEmpty())
		Expect(cfRouteStatus.Destinations).ToNot(BeEmpty())
		Expect(cfRouteStatus.CurrentStatus).To(Equal(korifiv1alpha1.ValidStatus))
		Expect(validCondition.Status).To(Equal(metav1.ConditionTrue))
	} else {
		Expect(cfRouteStatus.FQDN).To(BeEmpty())
		Expect(cfRouteStatus.URI).To(BeEmpty())
		Expect(cfRouteStatus.Destinations).To(BeEmpty())
		Expect(cfRouteStatus.CurrentStatus).To(Equal(korifiv1alpha1.InvalidStatus))
		Expect(validCondition.Status).To(Equal(metav1.ConditionFalse))
	}

	if len(desc) == 0 {
		Expect(cfRouteStatus.Description).NotTo(BeEmpty())
	} else {
		Expect(cfRouteStatus.Description).To(Equal(desc[0]))
	}
	reasonMatcher := Not(BeEmpty())
	messageMatcher := Not(BeEmpty())
	if len(desc) > 1 {
		reasonMatcher = Equal(desc[1])
	}
	if len(desc) > 2 {
		messageMatcher = Equal(desc[2])
	}

	Expect(validCondition.Reason).To(reasonMatcher)
	Expect(validCondition.Message).To(messageMatcher)
}
