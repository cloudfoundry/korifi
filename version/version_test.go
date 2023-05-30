package version_test

import (
	"code.cloudfoundry.org/korifi/version"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Version comparison", func() {
	var (
		objectVersion string
		checker       version.Checker
		obj           client.Object
		res           bool
		err           error
	)

	BeforeEach(func() {
		objectVersion = ""
	})

	JustBeforeEach(func() {
		checker = version.NewChecker("v0.7.1")
		obj = &v1.Pod{ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				version.KorifiCreationVersionKey: objectVersion,
			},
		}}
		res, err = checker.ObjectIsNewer(obj)
	})

	When("object version is greater", func() {
		BeforeEach(func() {
			objectVersion = "v1.0.0"
		})

		It("returns true", func() {
			Expect(res).To(BeTrue())
		})
	})

	When("object version is less", func() {
		BeforeEach(func() {
			objectVersion = "v0.7.0"
		})

		It("returns false", func() {
			Expect(res).To(BeFalse())
		})
	})

	When("object version is the same", func() {
		BeforeEach(func() {
			objectVersion = "v0.7.1"
		})

		It("returns false", func() {
			Expect(res).To(BeFalse())
		})
	})

	When("object version is incorrect", func() {
		BeforeEach(func() {
			objectVersion = "foo"
		})

		It("returns an error", func() {
			Expect(err).To(HaveOccurred())
		})
	})

	When("object version is empty", func() {
		BeforeEach(func() {
			objectVersion = ""
		})

		It("returns an error", func() {
			Expect(err).To(HaveOccurred())
		})
	})
})

var _ = Describe("construction", func() {
	When("checker version is incorrect", func() {
		It("panics on construction", func() {
			Expect(func() { version.NewChecker("blah") }).To(Panic())
		})
	})
})
