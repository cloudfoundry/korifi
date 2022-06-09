package jobs_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
)

func TestJob(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Job Suite")
}

func expectedValFrom(fieldPath string) *v1.EnvVarSource {
	return &v1.EnvVarSource{
		FieldRef: &v1.ObjectFieldSelector{
			APIVersion: "",
			FieldPath:  fieldPath,
		},
	}
}

var ctx context.Context

var _ = BeforeEach(func() {
	ctx = context.Background()
})
