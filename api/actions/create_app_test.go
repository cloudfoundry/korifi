package actions_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "code.cloudfoundry.org/cf-k8s-controllers/api/actions"
	"code.cloudfoundry.org/cf-k8s-controllers/api/actions/fake"
	"code.cloudfoundry.org/cf-k8s-controllers/api/payloads"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories"
)

var _ = Describe("CreateApp", func() {
	const (
		appGUID                   = "app-guid"
		appEtcdUID                = "app-etcd-uid"
		appName                   = "app-name"
		spaceGUID                 = "space-guid"
		createEnvVarsResponseName = "testAppGUID-env"
	)
	var (
		action    func(context.Context, client.Client, payloads.AppCreate) (repositories.AppRecord, error)
		appRepo   *fake.CFAppRepository
		client    *fake.Client
		payload   payloads.AppCreate
		appRecord repositories.AppRecord
	)

	BeforeEach(func() {
		appRepo = new(fake.CFAppRepository)
		action = NewCreateApp(appRepo).Invoke
		client = new(fake.Client)

		payload = payloads.AppCreate{
			Name:                 appName,
			EnvironmentVariables: nil,
			Relationships: payloads.AppRelationships{
				Space: payloads.Relationship{
					Data: &payloads.RelationshipData{
						GUID: spaceGUID,
					},
				},
			},
			Lifecycle: nil,
			Metadata:  payloads.Metadata{},
		}

		appRecord = repositories.AppRecord{
			GUID:      appGUID,
			EtcdUID:   appEtcdUID,
			Name:      appName,
			SpaceGUID: spaceGUID,
			State:     "STOPPED",
			Lifecycle: repositories.Lifecycle{
				Type: "buildpack",
				Data: repositories.LifecycleData{
					Buildpacks: []string{},
					Stack:      "cflinuxfs3",
				},
			},
		}

		appRepo.CreateAppReturns(appRecord, nil)
		appRepo.CreateOrPatchAppEnvVarsReturns(repositories.AppEnvVarsRecord{
			Name: createEnvVarsResponseName,
		}, nil)
	})

	When("there are no env vars or metadata", func() {
		BeforeEach(func() {
			payload.EnvironmentVariables = nil
			payload.Metadata = payloads.Metadata{}
		})

		It("returns the AppRecord without erroring", func() {
			result, err := action(context.Background(), client, payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(appRecord))
		})

		It("invokes repo CreateApp", func() {
			_, _ = action(context.Background(), client, payload)

			Expect(appRepo.CreateAppCallCount()).To(Equal(1))

			_, _, message := appRepo.CreateAppArgsForCall(0)
			Expect(message).To(Equal(repositories.AppCreateMessage{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				Labels:      nil,
				Annotations: nil,
				State:       "STOPPED",
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Stack: "cflinuxfs3",
					},
				},
				EnvironmentVariables: nil,
			}))
		})
	})

	When("env vars are set", func() {
		var (
			testEnvironmentVariables map[string]string
		)

		BeforeEach(func() {
			testEnvironmentVariables = map[string]string{"foo": "foo", "bar": "bar"}
			payload.EnvironmentVariables = testEnvironmentVariables
		})

		It("returns the AppRecord without erroring", func() {
			result, err := action(context.Background(), client, payload)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(appRecord))
		})

		It("invokes repo CreateApp", func() {
			_, _ = action(context.Background(), client, payload)

			Expect(appRepo.CreateAppCallCount()).To(Equal(1))

			_, _, message := appRepo.CreateAppArgsForCall(0)
			Expect(message).To(Equal(repositories.AppCreateMessage{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				Labels:      nil,
				Annotations: nil,
				State:       "STOPPED",
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Stack: "cflinuxfs3",
					},
				},
				EnvironmentVariables: testEnvironmentVariables,
			}))
		})
	})

	When("metadata labels are set", func() {
		var testLabels map[string]string

		BeforeEach(func() {
			testLabels = map[string]string{"foo": "foo", "bar": "bar"}
			payload.Metadata.Labels = testLabels
		})

		It("passes along the labels to CreateApp", func() {
			_, _ = action(context.Background(), client, payload)

			Expect(appRepo.CreateAppCallCount()).To(Equal(1))

			_, _, message := appRepo.CreateAppArgsForCall(0)
			Expect(message).To(Equal(repositories.AppCreateMessage{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				Labels:      testLabels,
				Annotations: nil,
				State:       "STOPPED",
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Stack: "cflinuxfs3",
					},
				},
			}))
		})
	})

	When("metadata annotations are set", func() {
		var testAnnotations map[string]string

		BeforeEach(func() {
			testAnnotations = map[string]string{"foo": "foo", "bar": "bar"}
			payload.Metadata.Annotations = testAnnotations
		})

		It("passes along the annotation to CreateApp", func() {
			_, _ = action(context.Background(), client, payload)

			Expect(appRepo.CreateAppCallCount()).To(Equal(1))

			_, _, message := appRepo.CreateAppArgsForCall(0)
			Expect(message).To(Equal(repositories.AppCreateMessage{
				Name:        appName,
				SpaceGUID:   spaceGUID,
				Labels:      nil,
				Annotations: testAnnotations,
				State:       "STOPPED",
				Lifecycle: repositories.Lifecycle{
					Type: "buildpack",
					Data: repositories.LifecycleData{
						Stack: "cflinuxfs3",
					},
				},
			}))
		})
	})
})
