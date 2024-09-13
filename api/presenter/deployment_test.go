package presenter_test

import (
	"encoding/json"
	"net/url"
	"time"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/tools"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Deployments", func() {
	var (
		baseURL *url.URL
		output  []byte
		record  repositories.DeploymentRecord
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		record = repositories.DeploymentRecord{
			GUID:        "app-guid",
			DropletGUID: "droplet-guid",
			CreatedAt:   time.UnixMilli(1000),
			UpdatedAt:   tools.PtrTo(time.UnixMilli(2000)),
			Status: repositories.DeploymentStatus{
				Value:  "deployment-status-value",
				Reason: "deployment-status-reason",
			},
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForDeployment(record, *baseURL)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces expected deployment json", func() {
		Expect(output).To(MatchJSON(`{
			"guid": "app-guid",
			"status": {
				"value": "deployment-status-value",
				"reason": "deployment-status-reason"
			},
			"droplet": {
				"guid": "droplet-guid"
			},
			"relationships": {
				"app": {
					"data": {
						"guid": "app-guid"
					}
				}
			},
			"created_at": "1970-01-01T00:00:01Z",
			"updated_at": "1970-01-01T00:00:02Z",
			"links": {
				"self": {
					"href": "https://api.example.org/v3/deployments/app-guid"
				},
				"app": {
					"href": "https://api.example.org/v3/apps/app-guid"
				}
			}
		}`))
	})
})
