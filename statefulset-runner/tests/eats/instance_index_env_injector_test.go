package eats_test

import (
	"context"
	"regexp"

	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("InstanceIndexEnvInjector [needs-logs-for: eirini-api, instance-index-env-injector]", func() {
	var (
		lrpGUID        string
		appServiceName string
	)

	BeforeEach(func() {
		lrpGUID = tests.GenerateGUID()

		lrp := &eiriniv1.LRP{
			ObjectMeta: metav1.ObjectMeta{
				Name: tests.GenerateGUID(),
			},
			Spec: eiriniv1.LRPSpec{
				GUID:                   lrpGUID,
				Version:                tests.GenerateGUID(),
				Image:                  "eirini/dorini",
				AppGUID:                "the-app-guid",
				AppName:                "k-2so",
				SpaceName:              "s",
				OrgName:                "o",
				Env:                    map[string]string{"FOO": "BAR"},
				MemoryMB:               256,
				DiskMB:                 256,
				CPUWeight:              10,
				Instances:              3,
				Ports:                  []int32{8080},
				VolumeMounts:           []eiriniv1.VolumeMount{},
				UserDefinedAnnotations: map[string]string{},
			},
		}

		_, err := fixture.EiriniClientset.
			EiriniV1().
			LRPs(fixture.Namespace).
			Create(context.Background(), lrp, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		appServiceName = tests.ExposeAsService(fixture.Clientset, fixture.Namespace, lrpGUID, 8080, "/")
	})

	It("creates pods with CF_INSTANCE_INDEX set to 0, 1 and 2", func() {
		guids := map[string]bool{}
		re := regexp.MustCompile(`CF_INSTANCE_INDEX=(.*)`)
		Eventually(func() int {
			envvars, err := tests.RequestServiceFn(fixture.Namespace, appServiceName, 8080, "/env")()
			if err != nil {
				return 0
			}
			matches := re.FindStringSubmatch(envvars)
			if len(matches) == 2 {
				guids[matches[1]] = true
			}

			return len(guids)
		}).Should(Equal(3))

		Expect(guids).To(And(HaveKey("0"), HaveKey("1"), HaveKey("2")))
	})
})
