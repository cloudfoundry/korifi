package k8s_test

import (
	"code.cloudfoundry.org/korifi/tools/k8s"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("MatchNothingSelector", func() {
	It("mathes nothing", func() {
		nsList := &corev1.NamespaceList{}
		Expect(k8sClient.List(ctx, nsList, client.MatchingLabelsSelector{Selector: k8s.MatchNotingSelector()})).To(Succeed())
		Expect(nsList.Items).To(BeEmpty())
	})
})
