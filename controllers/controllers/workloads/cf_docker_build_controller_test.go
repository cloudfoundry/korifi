package workloads_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/dockercfg"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("CFDockerBuildReconciler Integration Tests", func() {
	var (
		imageSecret *corev1.Secret
		imageConfig *v1.ConfigFile
		imageRef    string

		cfSpace       *korifiv1alpha1.CFSpace
		cfApp         *korifiv1alpha1.CFApp
		cfPackageGUID string
		cfBuild       *korifiv1alpha1.CFBuild
	)

	BeforeEach(func() {
		imageRef = containerRegistry.ImageRef("foo/bar")
		imageConfig = &v1.ConfigFile{
			Config: v1.Config{
				User: "1000",
			},
		}

		cfSpace = createSpace(cfOrg)

		var err error
		imageSecret, err = dockercfg.CreateDockerConfigSecret(
			cfSpace.Status.GUID,
			PrefixedGUID("image-secret"),
			dockercfg.DockerServerConfig{
				Server:   containerRegistry.URL(),
				Username: "user",
				Password: "password",
			},
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(adminClient.Create(ctx, imageSecret)).To(Succeed())

		cfApp = &korifiv1alpha1.CFApp{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PrefixedGUID("cf-app"),
				Namespace: cfSpace.Status.GUID,
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  PrefixedGUID("cf-app-display-name"),
				DesiredState: "STOPPED",
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "docker",
					Data: korifiv1alpha1.LifecycleData{},
				},
			},
		}
		Expect(adminClient.Create(ctx, cfApp)).To(Succeed())

		cfPackageGUID = PrefixedGUID("cf-package")
		Expect(adminClient.Create(ctx, &korifiv1alpha1.CFPackage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfPackageGUID,
				Namespace: cfSpace.Status.GUID,
			},
			Spec: korifiv1alpha1.CFPackageSpec{
				Type: "docker",
				AppRef: corev1.LocalObjectReference{
					Name: cfApp.Name,
				},
				Source: korifiv1alpha1.PackageSource{
					Registry: korifiv1alpha1.Registry{
						Image:            imageRef,
						ImagePullSecrets: []corev1.LocalObjectReference{{Name: imageSecret.Name}},
					},
				},
			},
		})).To(Succeed())

		cfBuild = &korifiv1alpha1.CFBuild{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PrefixedGUID("cf-build"),
				Namespace: cfSpace.Status.GUID,
			},
			Spec: korifiv1alpha1.CFBuildSpec{
				PackageRef: corev1.LocalObjectReference{
					Name: cfPackageGUID,
				},
				AppRef: corev1.LocalObjectReference{
					Name: cfApp.Name,
				},
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "docker",
				},
			},
		}
	})

	JustBeforeEach(func() {
		containerRegistry.PushImage(containerRegistry.ImageRef("foo/bar"), imageConfig)
		Expect(adminClient.Create(ctx, cfBuild)).To(Succeed())
	})

	It("sets the observed generation in the status", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
			g.Expect(cfBuild.Status.ObservedGeneration).To(Equal(cfBuild.Generation))
		}).Should(Succeed())
	})

	It("sets the app as build owner", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
			g.Expect(cfBuild.GetOwnerReferences()).To(ConsistOf(
				metav1.OwnerReference{
					APIVersion:         korifiv1alpha1.GroupVersion.Identifier(),
					Kind:               "CFApp",
					Name:               cfApp.Name,
					UID:                cfApp.UID,
					Controller:         tools.PtrTo(true),
					BlockOwnerDeletion: tools.PtrTo(true),
				},
			))
		}).Should(Succeed())
	})

	It("cleans up older builds and droplets", func() {
		Eventually(func(g Gomega) {
			found := false
			for i := 0; i < buildCleaner.CleanCallCount(); i++ {
				_, app := buildCleaner.CleanArgsForCall(i)
				if app.Name == cfApp.Name && app.Namespace == cfSpace.Status.GUID {
					found = true
					break
				}
			}
			g.Expect(found).To(BeTrue())
		}).Should(Succeed())
	})

	It("makes the build succeed", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
			g.Expect(meta.IsStatusConditionFalse(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)).To(BeTrue())
			g.Expect(meta.IsStatusConditionTrue(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)).To(BeTrue())
			g.Expect(cfBuild.Status.Droplet).NotTo(BeNil())
			g.Expect(cfBuild.Status.Droplet.Registry.Image).To(Equal(imageRef))
			g.Expect(cfBuild.Status.Droplet.Registry.ImagePullSecrets).To(ConsistOf(corev1.LocalObjectReference{Name: imageSecret.Name}))
		}).Should(Succeed())
	})

	Describe("privileged images", func() {
		succeededCondition := func(g Gomega) metav1.Condition {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
			g.Expect(meta.IsStatusConditionFalse(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)).To(BeTrue())
			succeedCondition := meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)
			g.Expect(succeedCondition).NotTo(BeNil())

			return *succeedCondition
		}

		haveFailed := gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Status":  Equal(metav1.ConditionFalse),
			"Reason":  Equal("BuildFailed"),
			"Message": ContainSubstring("not supported"),
		})

		When("the user is not specified", func() {
			BeforeEach(func() {
				imageConfig.Config.User = ""
			})

			It("fails the build", func() {
				Eventually(succeededCondition).Should(haveFailed)
			})
		})

		When("the user is 'root'", func() {
			BeforeEach(func() {
				imageConfig.Config.User = "root"
			})

			It("fails the build", func() {
				Eventually(succeededCondition).Should(haveFailed)
			})
		})

		When("the user is '0'", func() {
			BeforeEach(func() {
				imageConfig.Config.User = "0"
			})

			It("fails the build", func() {
				Eventually(succeededCondition).Should(haveFailed)
			})
		})

		When("the user is 'root:rootgroup'", func() {
			BeforeEach(func() {
				imageConfig.Config.User = "root:rootgroup"
			})

			It("fails the build", func() {
				Eventually(succeededCondition).Should(haveFailed)
			})
		})

		When("the user is '0:rootgroup'", func() {
			BeforeEach(func() {
				imageConfig.Config.User = "0:rootgroup"
			})

			It("fails the build", func() {
				Eventually(succeededCondition).Should(haveFailed)
			})
		})
	})
})
