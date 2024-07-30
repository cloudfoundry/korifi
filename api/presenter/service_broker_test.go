package presenter_test

import (
	"encoding/json"
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/model"
	"code.cloudfoundry.org/korifi/model/services"
	"code.cloudfoundry.org/korifi/tools"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Service Broker", func() {
	var (
		baseURL *url.URL
		output  []byte
		record  repositories.ServiceBrokerRecord
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		record = repositories.ServiceBrokerRecord{
			ServiceBroker: services.ServiceBroker{
				Name: "my-broker",
				URL:  "https://my.broker",
			},
			CFResource: model.CFResource{
				GUID:      "resource-guid",
				CreatedAt: time.UnixMilli(1000),
				UpdatedAt: tools.PtrTo(time.UnixMilli(2000)),
				Metadata: model.Metadata{
					Labels: map[string]string{
						"label": "broker-label",
					},
					Annotations: map[string]string{
						"annotation": "broker-annotation",
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForServiceBroker(record, *baseURL)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns the expected JSON", func() {
		Expect(output).To(MatchJSON(`{
			"name": "my-broker",
			"url": "https://my.broker",
			"guid": "resource-guid",
			"created_at": "1970-01-01T00:00:01Z",
			"updated_at": "1970-01-01T00:00:02Z",
			"metadata": {
			  "labels": {
				"label": "broker-label"
			  },
			  "annotations": {
				"annotation": "broker-annotation"
			  }
			},
			"links": {
			  "self": {
				"href": "https://api.example.org/v3/service_brokers/resource-guid"
			  },
			  "service_offerings": {
				"href": "https://api.example.org/v3/service_offerings?service_broker_guids=resource-guid"
			  }
			}
		}`))
	})
})
