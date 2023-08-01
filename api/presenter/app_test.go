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

var _ = Describe("App", func() {
	var (
		baseURL *url.URL
		output  []byte
	)

	BeforeEach(func() {
		var err error
		baseURL, err = url.Parse("https://api.example.org")
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("App Response", func() {
		var record repositories.AppRecord

		BeforeEach(func() {
			record = repositories.AppRecord{
				Name:        "test-app",
				GUID:        "app-guid",
				SpaceGUID:   "space-guid",
				Labels:      map[string]string{"label-key": "label-value"},
				Annotations: map[string]string{"annotation-key": "annotation-value"},
				State:       "STOPPED",
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Buildpacks: []string{"foo", "bar"},
						Stack:      "cflinuxfs2",
					},
				},
				CreatedAt: time.UnixMilli(1000),
				UpdatedAt: tools.PtrTo(time.UnixMilli(2000)),
				IsStaged:  false,
			}
		})

		JustBeforeEach(func() {
			response := presenter.ForApp(record, *baseURL)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("produces expected app json", func() {
			Expect(output).To(MatchJSON(`{
				"guid": "app-guid",
				"created_at": "1970-01-01T00:00:01Z",
				"updated_at": "1970-01-01T00:00:02Z",
				"name": "test-app",
				"state": "STOPPED",
				"lifecycle": {
					"type": "buildpack",
					"data": {
						"buildpacks": ["foo", "bar"],
						"stack": "cflinuxfs2"
					}
				},
				"relationships": {
					"space": {
						"data": {
							"guid": "space-guid"
						}
					}
				},
				"metadata": {
					"labels": {
						"label-key": "label-value"
					},
					"annotations": {
						"annotation-key": "annotation-value"
					}
				},
				"links": {
					"self": {
						"href": "https://api.example.org/v3/apps/app-guid"
					},
					"environment_variables": {
						"href": "https://api.example.org/v3/apps/app-guid/environment_variables"
					},
					"space": {
						"href": "https://api.example.org/v3/spaces/space-guid"
					},
					"processes": {
						"href": "https://api.example.org/v3/apps/app-guid/processes"
					},
					"packages": {
						"href": "https://api.example.org/v3/apps/app-guid/packages"
					},
					"current_droplet": {
						"href": "https://api.example.org/v3/apps/app-guid/droplets/current"
					},
					"droplets": {
						"href": "https://api.example.org/v3/apps/app-guid/droplets"
					},
					"tasks": {
						"href": "https://api.example.org/v3/apps/app-guid/tasks"
					},
					"start": {
						"href": "https://api.example.org/v3/apps/app-guid/actions/start",
						"method": "POST"
					},
					"stop": {
						"href": "https://api.example.org/v3/apps/app-guid/actions/stop",
						"method": "POST"
					},
					"revisions": {
						"href": "https://api.example.org/v3/apps/app-guid/revisions"
					},
					"deployed_revisions": {
						"href": "https://api.example.org/v3/apps/app-guid/revisions/deployed"
					},
					"features": {
						"href": "https://api.example.org/v3/apps/app-guid/features"
					}
				}
			} `))
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

		When("buildpacks is not set", func() {
			BeforeEach(func() {
				record.Lifecycle.Data.Buildpacks = nil
			})

			It("renders an empty list", func() {
				Expect(string(output)).To(ContainSubstring(`"buildpacks":[]`))
			})
		})
	})

	Describe("Droplet Response", func() {
		var record repositories.CurrentDropletRecord

		BeforeEach(func() {
			record = repositories.CurrentDropletRecord{
				AppGUID:     "app-guid",
				DropletGUID: "droplet-guid",
			}
		})

		JustBeforeEach(func() {
			response := presenter.ForCurrentDroplet(record, *baseURL)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected JSON", func() {
			Expect(output).To(MatchJSON(`{
				"data": {
					"guid": "droplet-guid"
				},
				"links": {
					"self": {
						"href": "https://api.example.org/v3/apps/app-guid/relationships/current_droplet"
					},
					"related": {
						"href": "https://api.example.org/v3/apps/app-guid/droplets/current"
					}
				}
			}`))
		})
	})

	Describe("App Env", func() {
		var record repositories.AppEnvRecord

		BeforeEach(func() {
			record = repositories.AppEnvRecord{
				EnvironmentVariables: map[string]string{"VAR": "VAL"},
				SystemEnv: map[string]any{
					"VCAP_SERVICES": map[string]any{
						"mysql": map[string]any{
							"plan": "xlarge",
						},
					},
				},
				AppEnv: map[string]any{
					"VCAP_APPLICATION": map[string]any{
						"application_name": "my-app",
					},
				},
			}
		})

		JustBeforeEach(func() {
			response := presenter.ForAppEnv(record)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected output", func() {
			Expect(output).To(MatchJSON(`{
				"staging_env_json": {},
				"running_env_json": {},
				"environment_variables": {
					"VAR": "VAL"
				},
				"system_env_json": {
					"VCAP_SERVICES": {
						"mysql": {
							"plan": "xlarge"
						}
					}
				},
				"application_env_json": {
					"VCAP_APPLICATION": {
						"application_name": "my-app"
					}
				}
			}`))
		})

		When("system env is nil", func() {
			BeforeEach(func() {
				record.SystemEnv = nil
			})

			It("returns an empty list", func() {
				Expect(output).To(MatchJSONPath("$.system_env_json", Not(BeNil())))
			})
		})

		When("app env is nil", func() {
			BeforeEach(func() {
				record.AppEnv = nil
			})

			It("returns an empty list", func() {
				Expect(output).To(MatchJSONPath("$.application_env_json", Not(BeNil())))
			})
		})
	})

	Describe("App Env Vars", func() {
		var record repositories.AppEnvVarsRecord

		BeforeEach(func() {
			record = repositories.AppEnvVarsRecord{
				Name:      "my-app-env",
				AppGUID:   "app-guid",
				SpaceGUID: "space-guid",
				EnvironmentVariables: map[string]string{
					"KEY0": "VAL0",
					"KEY2": "VAL2",
				},
			}
		})

		JustBeforeEach(func() {
			response := presenter.ForAppEnvVars(record, *baseURL)
			var err error
			output, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected output", func() {
			Expect(output).To(MatchJSON(`{
				"var": {
					"KEY0": "VAL0",
					"KEY2": "VAL2"
				},
				"links": {
					"self": {
						"href": "https://api.example.org/v3/apps/app-guid/environment_variables"
					},
					"app": {
						"href": "https://api.example.org/v3/apps/app-guid"
					}
				}
			}`))
		})
	})
})
