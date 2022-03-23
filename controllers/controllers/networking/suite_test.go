package networking_test

import (
	"testing"

	networkingv1alpha1 "code.cloudfoundry.org/cf-k8s-controllers/controllers/apis/networking/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNetworkingControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Networking Controllers Unit Test Suite")
}

func expectCFRouteValidStatus(cfRouteStatus networkingv1alpha1.CFRouteStatus, valid bool, desc ...string) {
	expectedCurrentStatus := networkingv1alpha1.ValidStatus
	expectedValidStatusCondition := metav1.ConditionTrue
	if !valid {
		expectedCurrentStatus = networkingv1alpha1.InvalidStatus
		expectedValidStatusCondition = metav1.ConditionFalse
	}
	Expect(cfRouteStatus.CurrentStatus).To(Equal(expectedCurrentStatus))

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

	validStatus := meta.FindStatusCondition(cfRouteStatus.Conditions, "Valid")
	Expect(validStatus).NotTo(BeNil())
	Expect(*validStatus).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("Valid"),
		"Status":  Equal(expectedValidStatusCondition),
		"Reason":  reasonMatcher,
		"Message": messageMatcher,
	}))
}
