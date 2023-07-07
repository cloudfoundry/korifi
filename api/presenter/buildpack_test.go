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

var _ = Describe("Buildpacks", func() {
	var (
		output []byte
		record repositories.BuildpackRecord
	)

	BeforeEach(func() {
		record = repositories.BuildpackRecord{
			Name:      "paketo-foopacks/bar",
			Position:  1,
			Stack:     "waffle-house",
			Version:   "1.0.0",
			CreatedAt: time.UnixMilli(1000),
			UpdatedAt: tools.PtrTo(time.UnixMilli(2000)),
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForBuildpack(record, url.URL{})
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces expected build json", func() {
		Expect(output).To(MatchJSON(`{
			"guid": "",
			"created_at": "1970-01-01T00:00:01Z",
			"updated_at": "1970-01-01T00:00:02Z",
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
