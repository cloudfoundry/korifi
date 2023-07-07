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

var _ = Describe("Package", func() {
	var (
		baseURL *url.URL
		output  []byte
		record  repositories.PackageRecord
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())

		record = repositories.PackageRecord{
			GUID:      "the-package-guid",
			Type:      "bits",
			AppGUID:   "the-app-guid",
			SpaceGUID: "the-space-guid",
			State:     "AWAITING_UPLOAD",
			CreatedAt: time.UnixMilli(1000),
			UpdatedAt: tools.PtrTo(time.UnixMilli(2000)),
			Labels: map[string]string{
				"foo": "bar",
			},
			Annotations: map[string]string{
				"baz": "fof",
			},
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForPackage(record, *baseURL)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("produces the expected json", func() {
		Expect(output).To(MatchJSON(`{
			"guid": "the-package-guid",
			"type": "bits",
			"data": {},
			"state": "AWAITING_UPLOAD",
			"created_at": "1970-01-01T00:00:01Z",
			"updated_at": "1970-01-01T00:00:02Z",
			"relationships": {
				"app": {
					"data": {
						"guid": "the-app-guid"
					}
				}
			},
			"links": {
				"self": {
					"href": "https://api.example.org/v3/packages/the-package-guid"
				},
				"upload": {
					"href": "https://api.example.org/v3/packages/the-package-guid/upload",
					"method": "POST"
				},
				"download": {
					"href": "https://api.example.org/v3/packages/the-package-guid/download",
					"method": "GET"
				},
				"app": {
					"href": "https://api.example.org/v3/apps/the-app-guid"
				}
			},
			"metadata": {
				"labels": {
					"foo": "bar"
				},
				"annotations": {
					"baz": "fof"
				}
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
