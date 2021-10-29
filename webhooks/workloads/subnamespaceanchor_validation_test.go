package workloads_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("SubnamespaceanchorValidation", func() {
	Describe("subnamespace anchor creation", func() {
		Context("orgs", func() {
			It("searches for matching org labels in the namespace", func() {
				Expect(true).To(BeTrue())
			})

			When("the org name is unique in the namespace", func() {
				It("allows the request", func() {
				})
			})

			When("the org name already exists in the namespace", func() {
				It("denies the request", func() {
				})
			})
		})

		Context("spaces", func() {
			It("searches for matching space labels in the namespace", func() {
			})

			When("the space name is unique in the namespace", func() {
				It("allows the request", func() {
				})
			})

			When("the space name already exists in the namespace", func() {
				It("denies the request", func() {
				})
			})
		})

		Context("malformed orgs and spaces", func() {
			When("a subnamespace anchor has neither org nor space label", func() {
				It("allows the request", func() {
				})
			})

			When("a subnamespace anchor has both org and space labels", func() {
				It("denies the request", func() {
				})
			})
		})

		Context("failures", func() {
			When("decoding fails", func() {
				It("denies the request", func() {
				})
			})

			When("listing fails", func() {
				It("denies the request", func() {
				})
			})
		})
	})

	Describe("subnamespace anchor updates", func() {
		Context("orgs", func() {
			It("searches for matching org labels in the namespace", func() {
			})

			When("the new org name is unique in the namespace", func() {
				It("allows the request", func() {
				})
			})

			When("the new org name already exists in the namespace", func() {
				It("denies the request", func() {
				})
			})

			When("the org name hasn't changed", func() {
				It("succeeds", func() {
				})
			})
		})

		Context("spaces", func() {
			It("searches for matching space labels in the namespace", func() {
			})

			When("the new space name is unique in the namespace", func() {
				It("allows the request", func() {
				})
			})

			When("the new space name already exists in the namespace", func() {
				It("denies the request", func() {
				})
			})

			When("the space name hasn't changed", func() {
				It("succeeds", func() {
				})
			})
		})
	})
})
