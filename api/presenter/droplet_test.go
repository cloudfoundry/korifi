package presenter_test

import (
	"encoding/json"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Droplet", func() {
	var (
		baseURL *url.URL
		output  []byte
		record  repositories.DropletRecord
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		record = repositories.DropletRecord{
			GUID:      "the-droplet-guid",
			State:     "STAGED",
			CreatedAt: "2019-05-10T17:17:48Z",
			UpdatedAt: "2019-05-10T17:17:48Z",
			Lifecycle: repositories.Lifecycle{
				Type: "buildpack",
			},
			Stack: "cflinuxfs3",
			ProcessTypes: map[string]string{
				"rake": "bundle exec rake",
				"web":  "bundle exec rackup config.ru -p $PORT",
			},
			AppGUID:     "the-app-guid",
			PackageGUID: "the-package-guid",
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForDroplet(record, *baseURL)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces expected droplet json", func() {
		Expect(output).To(MatchJSON(`{
			"guid": "the-droplet-guid",
			"state": "STAGED",
			"error": null,
			"lifecycle": {
				"type": "buildpack",
				"data": {
					"buildpacks": [],
					"stack": ""
				}
			},
			"execution_metadata": "",
			"process_types": {
				"rake": "bundle exec rake",
				"web": "bundle exec rackup config.ru -p $PORT"
			},
			"checksum": null,
			"buildpacks": [],
			"stack": "cflinuxfs3",
			"image": null,
			"created_at": "2019-05-10T17:17:48Z",
			"updated_at": "2019-05-10T17:17:48Z",
			"relationships": {
				"app": {
					"data": {
						"guid": "the-app-guid"
					}
				}
			},
			"links": {
				"self": {
					"href": "https://api.example.org/v3/droplets/the-droplet-guid"
				},
				"package": {
					"href": "https://api.example.org/v3/packages/the-package-guid"
				},
				"app": {
					"href": "https://api.example.org/v3/apps/the-app-guid"
				},
				"assign_current_droplet": {
					"href": "https://api.example.org/v3/apps/the-app-guid/relationships/current_droplet",
					"method": "PATCH"
				},
				"download": null
			},
			"metadata": {
				"labels": {},
				"annotations": {}
			}
		}`))
	})
})
