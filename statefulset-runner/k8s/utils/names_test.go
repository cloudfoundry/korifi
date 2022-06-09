package utils_test

import (
	. "code.cloudfoundry.org/korifi/statefulset-runner/k8s/utils"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Names", func() {
	Describe("GetStatefulsetName", func() {
		It("calculates the name of an app's backing statefulset", func() {
			statefulsetName, err := GetStatefulsetName(&eiriniv1.LRP{
				Spec: eiriniv1.LRPSpec{
					GUID:      "guid",
					Version:   "version",
					AppName:   "app",
					SpaceName: "space",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(statefulsetName).To(Equal("app-space-077dc99e95"))
		})

		When("the prefix is too long", func() {
			It("calculates the name of an app's backing statefulset", func() {
				statefulsetName, err := GetStatefulsetName(&eiriniv1.LRP{
					Spec: eiriniv1.LRPSpec{
						GUID:      "guid",
						Version:   "version",
						AppName:   "very-long-app-name",
						SpaceName: "space-with-very-very-very-very-very-very-very-very-very-long-name",
					},
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(statefulsetName).To(Equal("very-long-app-name-space-with-very-very--077dc99e95"))
			})
		})
	})

	Describe("GetJobName", func() {
		It("calculates the name of a task's backing job", func() {
			jobName := GetJobName(&eiriniv1.Task{
				Spec: eiriniv1.TaskSpec{
					GUID:      "guid",
					AppName:   "app",
					SpaceName: "space",
				},
			})
			Expect(jobName).To(Equal("app-space"))
		})

		When("the the task has a name", func() {
			It("calculates the name of a task's backing job", func() {
				jobName := GetJobName(&eiriniv1.Task{
					Spec: eiriniv1.TaskSpec{
						GUID:      "guid",
						Name:      "foo",
						AppName:   "app",
						SpaceName: "space",
					},
				})
				Expect(jobName).To(Equal("app-space-foo"))
			})
		})

		When("the prefix is too long", func() {
			It("calculates the name of an task's backing job", func() {
				jobName := GetJobName(&eiriniv1.Task{
					Spec: eiriniv1.TaskSpec{
						GUID:      "guid",
						AppName:   "very-long-app-name",
						SpaceName: "space-with-very-very-very-very-very-very-very-very-very-long-name",
					},
				})
				Expect(jobName).To(Equal("guid"))
			})
		})
	})
})
