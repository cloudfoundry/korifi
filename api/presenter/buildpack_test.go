package presenter_test

import (
	"encoding/json"
	"net/url"

	"code.cloudfoundry.org/korifi/api/presenter"
	"code.cloudfoundry.org/korifi/api/repositories"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Buildpacks", func() {
	var (
		baseURL *url.URL
		output  []byte
		record  repositories.BuildpackRecord
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		record = repositories.BuildpackRecord{
			Name:      "paketo-foopacks/bar",
			Position:  1,
			Stack:     "waffle-house",
			Version:   "1.0.0",
			CreatedAt: "2016-03-18T23:26:46Z",
			UpdatedAt: "2016-10-17T20:00:42Z",
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForBuildpack(record, *baseURL)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces expected build json", func() {
		Expect(output).To(MatchJSON(`{
			"guid": "",
			"created_at": "2016-03-18T23:26:46Z",
			"updated_at": "2016-10-17T20:00:42Z",
			"name": "paketo-foopacks/bar",
			"filename": "paketo-foopacks/bar@1.0.0",
			"stack": "waffle-house",
			"position": 1,
			"enabled": true,
			"locked": false,
			"metadata": {
				"labels": {},
				"annotations": {}
			},
			"links": {}
		}`))
	})
})
