package presenter_test

import (
	"encoding/json"
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Domains", func() {
	var (
		baseURL *url.URL
		output  []byte
		record  repositories.DomainRecord
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		record = repositories.DomainRecord{
			Name:        "my.domain",
			GUID:        "domain-guid",
			Labels:      map[string]string{"foo": "bar"},
			Annotations: map[string]string{"bar": "baz"},
			Namespace:   "my-ns",
			CreatedAt:   time.UnixMilli(1000),
			UpdatedAt:   tools.PtrTo(time.UnixMilli(2000)),
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForDomain(record, *baseURL)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces expected build json", func() {
		Expect(output).To(MatchJSON(`{
			"name": "my.domain",
			"guid": "domain-guid",
			"internal": false,
			"router_group": null,
			"supported_protocols": [
				"http"
			],
			"created_at": "1970-01-01T00:00:01Z",
			"updated_at": "1970-01-01T00:00:02Z",
			"metadata": {
				"labels": {
					"foo": "bar"
				},
				"annotations": {
					"bar": "baz"
				}
			},
			"relationships": {
				"organization": {
					"data": null
				},
				"shared_organizations": {
					"data": []
				}
			},
			"links": {
				"self": {
					"href": "https://api.example.org/v3/domains/domain-guid"
				},
				"route_reservations": {
					"href": "https://api.example.org/v3/domains/domain-guid/route_reservations"
				},
				"router_group": null
			}
		}`))
	})

	When("labels is nil", func() {
		BeforeEach(func() {
			record.Labels = nil
		})

		It("returns an empty slice of labels", func() {
			Expect(output).To(MatchJSONPath("$.metadata.labels", Not(BeNil())))
		})
	})

	When("annotations is nil", func() {
		BeforeEach(func() {
			record.Annotations = nil
		})

		It("returns an empty slice of annotations", func() {
			Expect(output).To(MatchJSONPath("$.metadata.annotations", Not(BeNil())))
		})
	})
})
