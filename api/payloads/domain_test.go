package payloads

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DomainList", func() {
	Describe("ToMessage", func() {
		When("a single name is specified", func() {
			It("properly splits them in the message", func() {
				names := "example.com"
				payload := DomainList{Names: &names}

				Expect(payload.ToMessage().Names).To(Equal([]string{"example.com"}))
			})
		})

		When("multiple names are specified", func() {
			It("properly splits them in the message and truncates whitespace", func() {
				names := " example.com, example.org ,cloudfoundry.org "
				payload := DomainList{Names: &names}

				Expect(payload.ToMessage().Names).To(Equal([]string{"example.com", "example.org", "cloudfoundry.org"}))
			})
		})

		When("no names are specified", func() {
			It("sets Names to an empty array", func() {
				payload := DomainList{}

				Expect(payload.ToMessage().Names).To(Equal([]string{}))
				Expect(len(payload.ToMessage().Names)).To(Equal(0))
			})
		})
	})
})
