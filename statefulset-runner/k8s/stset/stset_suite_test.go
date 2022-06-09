package stset_test

import (
	"context"
	"testing"

	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStset(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Stset Suite")
}

var ctx context.Context

var _ = BeforeEach(func() {
	ctx = context.Background()
})

func createLRP(namespace, name string) *eiriniv1.LRP {
	return &eiriniv1.LRP{
		ObjectMeta: v1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		Spec: eiriniv1.LRPSpec{
			GUID:        "guid_1234",
			Version:     "version_1234",
			ProcessType: "worker",
			AppName:     name,
			AppGUID:     "premium_app_guid_1234",
			SpaceName:   "space-foo",
			SpaceGUID:   "space-guid",
			Instances:   1,
			OrgName:     "org-foo",
			OrgGUID:     "org-guid",
			Command: []string{
				"/bin/sh",
				"-c",
				"while true; do echo hello; sleep 10;done",
			},
			MemoryMB:  1024,
			DiskMB:    2048,
			CPUWeight: 2,
			Image:     "gcr.io/foo/bar",
			Ports:     []int32{8888, 9999},
			VolumeMounts: []eiriniv1.VolumeMount{
				{
					ClaimName: "some-claim",
					MountPath: "/some/path",
				},
			},
			UserDefinedAnnotations: map[string]string{
				"prometheus.io/scrape": "secret-value",
			},
		},
	}
}
