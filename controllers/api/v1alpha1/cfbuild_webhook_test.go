package v1alpha1_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("CFBuildMutatingWebhook", func() {
	var (
		cfBuild       *korifiv1alpha1.CFBuild
		cfAppGUID     string
		cfPackageGUID string
		cfBuildGUID   string
	)

	BeforeEach(func() {
		cfAppGUID = uuid.NewString()
		cfPackageGUID = uuid.NewString()
		cfBuildGUID = uuid.NewString()

		cfBuild = &korifiv1alpha1.CFBuild{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfBuildGUID,
				Namespace: namespace,
				Labels:    map[string]string{"foo": "bar"},
			},
			Spec: korifiv1alpha1.CFBuildSpec{
				PackageRef: v1.LocalObjectReference{
					Name: cfPackageGUID,
				},
				AppRef: v1.LocalObjectReference{
					Name: cfAppGUID,
				},
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "buildpack",
					Data: korifiv1alpha1.LifecycleData{
						Buildpacks: []string{"java-buildpack"},
						Stack:      "cflinuxfs3",
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		Expect(adminClient.Create(ctx, cfBuild)).To(Succeed())
	})

	It("sets labels with the guids of the related app and package", func() {
		Expect(cfBuild.Labels).To(HaveKeyWithValue(korifiv1alpha1.CFAppGUIDLabelKey, cfAppGUID))
		Expect(cfBuild.Labels).To(HaveKeyWithValue(korifiv1alpha1.CFPackageGUIDLabelKey, cfPackageGUID))
	})

	It("preserves all other labels", func() {
		Expect(cfBuild.Labels).To(HaveKeyWithValue("foo", "bar"))
	})
})
