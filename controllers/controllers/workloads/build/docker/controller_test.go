package docker_test

import (
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/dockercfg"
	"code.cloudfoundry.org/korifi/tools/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
)

var _ = Describe("CFDockerBuildReconciler Integration Tests", func() {
	var (
		imageSecret *corev1.Secret
		imageConfig *v1.ConfigFile
		imageRef    string

		cfApp     *korifiv1alpha1.CFApp
		cfPackage *korifiv1alpha1.CFPackage
		cfBuild   *korifiv1alpha1.CFBuild
	)

	BeforeEach(func() {
		imageRef = containerRegistry.ImageRef("foo/bar")
		imageConfig = &v1.ConfigFile{
			Config: v1.Config{
				User: "1000",
			},
		}

		var err error
		imageSecret, err = dockercfg.CreateDockerConfigSecret(
			testNamespace,
			uuid.NewString(),
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
				Name:      uuid.NewString(),
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFAppSpec{
				DisplayName:  uuid.NewString(),
				DesiredState: "STOPPED",
				Lifecycle: korifiv1alpha1.Lifecycle{
					Type: "docker",
					Data: korifiv1alpha1.LifecycleData{},
				},
			},
		}
		Expect(adminClient.Create(ctx, cfApp)).To(Succeed())

		cfPackage = &korifiv1alpha1.CFPackage{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
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
		}
		Expect(adminClient.Create(ctx, cfPackage)).To(Succeed())

		cfBuild = &korifiv1alpha1.CFBuild{
			ObjectMeta: metav1.ObjectMeta{
				Name:      uuid.NewString(),
				Namespace: testNamespace,
			},
			Spec: korifiv1alpha1.CFBuildSpec{
				PackageRef: corev1.LocalObjectReference{
					Name: cfPackage.Name,
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

	It("makes the build succeed", func() {
		Eventually(func(g Gomega) {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
			g.Expect(meta.IsStatusConditionFalse(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)).To(BeTrue())
			g.Expect(meta.IsStatusConditionTrue(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)).To(BeTrue())
			g.Expect(cfBuild.Status.Droplet).NotTo(BeNil())
			g.Expect(cfBuild.Status.Droplet.Registry.Image).To(Equal(imageRef))
			g.Expect(cfBuild.Status.Droplet.Registry.ImagePullSecrets).To(ConsistOf(corev1.LocalObjectReference{Name: imageSecret.Name}))
			g.Expect(cfBuild.Status.Droplet.Ports).To(BeEmpty())
			g.Expect(cfBuild.Status.State).To(Equal(korifiv1alpha1.BuildStateStaged))
		}).Should(Succeed())
	})

	When("the image specifies ExposedPorts in its config", func() {
		BeforeEach(func() {
			imageConfig.Config.ExposedPorts = map[string]struct{}{"8888": {}, "9999": {}}
		})

		It("sets them into the droplet", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)).To(BeTrue())
				g.Expect(cfBuild.Status.Droplet).NotTo(BeNil())
				g.Expect(cfBuild.Status.Droplet.Ports).To(ConsistOf(int32(8888), int32(9999)))
			}).Should(Succeed())
		})
	})

	When("the package references an image that does not exist", func() {
		BeforeEach(func() {
			Expect(k8s.PatchResource(ctx, adminClient, cfPackage, func() {
				cfPackage.Spec.Source.Registry.Image = containerRegistry.ImageRef("does-not/exist")
			})).To(Succeed())
		})

		It("fails the build", func() {
			Eventually(func(g Gomega) {
				g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())

				g.Expect(meta.IsStatusConditionFalse(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType)).To(BeTrue())
				succeededConditionMessage := meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.SucceededConditionType).Message
				g.Expect(succeededConditionMessage).To(ContainSubstring("Failed to fetch image"))

				g.Expect(meta.IsStatusConditionFalse(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)).To(BeTrue())
				stagingConditionMessage := meta.FindStatusCondition(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType).Message
				g.Expect(stagingConditionMessage).To(ContainSubstring("Failed to fetch image"))

				g.Expect(cfBuild.Status.State).To(Equal(korifiv1alpha1.BuildStateFailed))
			}).Should(Succeed())
		})
	})

	Describe("privileged images", func() {
		completedBuild := func(g Gomega) korifiv1alpha1.CFBuild {
			g.Expect(adminClient.Get(ctx, client.ObjectKeyFromObject(cfBuild), cfBuild)).To(Succeed())
			g.Expect(meta.IsStatusConditionFalse(cfBuild.Status.Conditions, korifiv1alpha1.StagingConditionType)).To(BeTrue())

			return *cfBuild
		}

		haveFailed := gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Status": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Conditions": ContainElement(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
					"Type":    Equal(korifiv1alpha1.SucceededConditionType),
					"Status":  Equal(metav1.ConditionFalse),
					"Reason":  Equal("BuildFailed"),
					"Message": ContainSubstring("not supported"),
				})),
				"State": Equal(korifiv1alpha1.BuildStateFailed),
			}),
		})

		When("the user is not specified", func() {
			BeforeEach(func() {
				imageConfig.Config.User = ""
			})

			It("fails the build", func() {
				Eventually(completedBuild).Should(haveFailed)
			})
		})

		When("the user is 'root'", func() {
			BeforeEach(func() {
				imageConfig.Config.User = "root"
			})

			It("fails the build", func() {
				Eventually(completedBuild).Should(haveFailed)
			})
		})

		When("the user is '0'", func() {
			BeforeEach(func() {
				imageConfig.Config.User = "0"
			})

			It("fails the build", func() {
				Eventually(completedBuild).Should(haveFailed)
			})
		})

		When("the user is 'root:rootgroup'", func() {
			BeforeEach(func() {
				imageConfig.Config.User = "root:rootgroup"
			})

			It("fails the build", func() {
				Eventually(completedBuild).Should(haveFailed)
			})
		})

		When("the user is '0:rootgroup'", func() {
			BeforeEach(func() {
				imageConfig.Config.User = "0:rootgroup"
			})

			It("fails the build", func() {
				Eventually(completedBuild).Should(haveFailed)
			})
		})
	})
})
