package presenter_test

import (
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/relationships"
)

type unsupportedResource struct{}

func (r *unsupportedResource) Relationships() map[string]string {
	return nil
}

var _ = Describe("Resource", func() {
	var (
		resourcePresenter *presenter.Resource
		resource          relationships.Resource
		presentedResource any
	)

	BeforeEach(func() {
		url, err := url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())

		resource = &unsupportedResource{}
		resourcePresenter = presenter.NewResource(*url)
	})

	JustBeforeEach(func() {
		presentedResource = resourcePresenter.PresentResource(resource)
	})

	It("returns the original resource", func() {
		Expect(presentedResource).To(BeAssignableToTypeOf(&unsupportedResource{}))
	})

	When("the resource is a service broker", func() {
		BeforeEach(func() {
			resource = repositories.ServiceBrokerRecord{}
		})

		It("returns presented broker", func() {
			Expect(presentedResource).To(BeAssignableToTypeOf(presenter.ServiceBrokerResponse{}))
		})
	})

	When("the resource is a service offering", func() {
		BeforeEach(func() {
			resource = repositories.ServiceOfferingRecord{}
		})

		It("returns presented broker", func() {
			Expect(presentedResource).To(BeAssignableToTypeOf(presenter.ServiceOfferingResponse{}))
		})
	})

	When("the resource is a service plan", func() {
		BeforeEach(func() {
			resource = repositories.ServicePlanRecord{}
		})

		It("returns presented broker", func() {
			Expect(presentedResource).To(BeAssignableToTypeOf(presenter.ServicePlanResponse{}))
		})
	})
})
