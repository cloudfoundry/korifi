package singleton_test

import (
	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/tools/singleton"
	"code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type testResource struct {
	Foo string
}

func (r testResource) GetResourceType() string {
	return "my-type"
}

var _ = Describe("Get", func() {
	var (
		objects []testResource
		result  testResource
		err     error
	)

	BeforeEach(func() {
		objects = []testResource{{Foo: "bar"}}
	})

	JustBeforeEach(func() {
		result, err = singleton.Get(objects)
	})

	It("returns the single object", func() {
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(testResource{Foo: "bar"}))
	})

	When("resources slice is empty", func() {
		BeforeEach(func() {
			objects = []testResource{}
		})

		It("returns a not found error", func() {
			Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.NotFoundError{}))
		})
	})

	When("resources slice contains more than one elements", func() {
		BeforeEach(func() {
			objects = []testResource{{}, {}}
		})

		It("returns an unprocessable entity error", func() {
			Expect(err).To(matchers.WrapErrorAssignableToTypeOf(apierrors.UnprocessableEntityError{}))
		})
	})
})
