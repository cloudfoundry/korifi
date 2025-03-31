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

var _ = Describe("SecurityGroup", func() {
	var (
		baseURL *url.URL
		output  []byte
		record  repositories.SecurityGroupRecord
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
		record = repositories.SecurityGroupRecord{
			GUID:      "security-group-guid",
			CreatedAt: time.UnixMilli(1000).UTC(),
			UpdatedAt: tools.PtrTo(time.UnixMilli(2000).UTC()),
			Name:      "my-security-group",
			GloballyEnabled: repositories.SecurityGroupWorkloads{
				Running: true,
				Staging: false,
			},
			Rules: []repositories.SecurityGroupRule{
				{
					Protocol:    "tcp",
					Ports:       "80",
					Destination: "192.168.1.1",
				},
			},
			RunningSpaces: []string{"space-1", "space-2"},
			StagingSpaces: []string{"space-3"},
		}
	})

	JustBeforeEach(func() {
		response := presenter.ForSecurityGroup(record, *baseURL)
		var err error
		output, err = json.Marshal(response)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns the expected JSON", func() {
		Expect(output).To(MatchJSON(`{
            "guid": "security-group-guid",
            "created_at": "1970-01-01T00:00:01Z",
            "updated_at": "1970-01-01T00:00:02Z",
            "name": "my-security-group",
            "globally_enabled": {
                "running": true,
                "staging": false
            },
            "rules": [
                {
                    "protocol": "tcp",
                    "ports": "80",
                    "destination": "192.168.1.1"
                }
            ],
            "relationships": {
                "running_spaces": {
                    "data": [
                        { "guid": "space-1" },
                        { "guid": "space-2" }
                    ]
                },
                "staging_spaces": {
                    "data": [
                        { "guid": "space-3" }
                    ]
                }
            },
            "links": {
                "self": {
                    "href": "https://api.example.org/v3/security_groups/security-group-guid"
                }
            }
        }`))
	})

	When("there are no rules and no spaces", func() {
		BeforeEach(func() {
			record.Rules = []repositories.SecurityGroupRule{}
			record.RunningSpaces = []string{}
			record.StagingSpaces = []string{}
		})

		It("returns empty arrays for rules and relationships", func() {
			Expect(output).To(MatchJSON(`{
                "guid": "security-group-guid",
                "created_at": "1970-01-01T00:00:01Z",
                "updated_at": "1970-01-01T00:00:02Z",
                "name": "my-security-group",
                "globally_enabled": {
                    "running": true,
                    "staging": false
                },
                "rules": [],
                "relationships": {
                    "running_spaces": {
                        "data": []
                    },
                    "staging_spaces": {
                        "data": []
                    }
                },
                "links": {
                    "self": {
                        "href": "https://api.example.org/v3/security_groups/security-group-guid"
                    }
                }
            }`))
		})
	})
})
