package controllers_test

import (
	"context"
	"encoding/base64"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	corev1alpha1 "github.com/pivotal/kpack/pkg/apis/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("BuildWorkloadReconciler", func() {
	const (
		succeededConditionType              = "Succeeded"
		kpackReadyConditionType             = "Ready"
		wellFormedRegistryCredentialsSecret = "image-registry-credentials"
		kpackReconcilerName                 = "kpack-image-builder"
	)

	var (
		namespaceGUID  string
		cfBuildGUID    string
		namespace      *corev1.Namespace
		buildWorkload  *korifiv1alpha1.BuildWorkload
		source         korifiv1alpha1.PackageSource
		env            []corev1.EnvVar
		services       []corev1.ObjectReference
		reconcilerName string
	)

	eventuallyKpackImageShould := func(assertion func(*buildv1alpha2.Image, Gomega)) {
		Eventually(func(g Gomega) {
			kpackImage := new(buildv1alpha2.Image)
			err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}, kpackImage)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(kpackImage.Spec.Build).ToNot(BeNil())
			assertion(kpackImage, g)
		}).Should(Succeed())
	}

	BeforeEach(func() {
		beforeCtx := context.Background()

		namespaceGUID = PrefixedGUID("namespace")
		namespace = buildNamespaceObject(namespaceGUID)
		Expect(k8sClient.Create(beforeCtx, namespace)).To(Succeed())

		dockerRegistrySecret := buildDockerRegistrySecret(wellFormedRegistryCredentialsSecret, namespaceGUID)
		Expect(k8sClient.Create(beforeCtx, dockerRegistrySecret)).To(Succeed())

		registryServiceAccountName := "kpack-service-account" // this name is assumed in the controller code
		registryServiceAccount := buildServiceAccount(registryServiceAccountName, namespaceGUID, wellFormedRegistryCredentialsSecret)
		Expect(k8sClient.Create(beforeCtx, registryServiceAccount)).To(Succeed())

		cfBuildGUID = PrefixedGUID("cf-build")
		env = []corev1.EnvVar{{
			Name: "VCAP_SERVICES",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "some-vcap-services-secret",
					},
					Key: "VCAP_SERVICES",
				},
			},
		}}
		services = []corev1.ObjectReference{
			{
				Kind: "Secret",
				Name: "some-services-secret",
			},
		}

		source = korifiv1alpha1.PackageSource{
			Registry: korifiv1alpha1.Registry{
				Image:            "PACKAGE_IMAGE",
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: wellFormedRegistryCredentialsSecret}},
			},
		}

		reconcilerName = kpackReconcilerName
	})

	AfterEach(func() {
		Expect(k8sClient.Delete(context.Background(), namespace)).To(Succeed())
	})

	When("BuildWorkload is first created", func() {
		JustBeforeEach(func() {
			buildWorkload = buildWorkloadObject(cfBuildGUID, namespaceGUID, source, env, services, reconcilerName)
			Expect(k8sClient.Create(context.Background(), buildWorkload)).To(Succeed())
		})

		It("creates a kpack image with the source, env and services set", func() {
			eventuallyKpackImageShould(func(kpackImage *buildv1alpha2.Image, g Gomega) {
				g.Expect(kpackImage.Spec.Source.Registry.Image).To(BeEquivalentTo(source.Registry.Image))
				g.Expect(kpackImage.Spec.Source.Registry.ImagePullSecrets).To(BeEquivalentTo(source.Registry.ImagePullSecrets))
				g.Expect(kpackImage.Spec.Build.Env).To(Equal(env))
				g.Expect(kpackImage.Spec.Build.Services).To(BeEquivalentTo(services))
			})
		})

		It("sets the status condition on BuildWorkload", func() {
			cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
			updatedBuildWorkload := new(korifiv1alpha1.BuildWorkload)
			Eventually(func(g Gomega) {
				err := k8sClient.Get(context.Background(), cfBuildLookupKey, updatedBuildWorkload)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(mustHaveCondition(g, updatedBuildWorkload.Status.Conditions, succeededConditionType).Status).To(Equal(metav1.ConditionUnknown))
			}).Should(Succeed())
		})

		When("kpack image already exists", func() {
			var existingKpackImage *buildv1alpha2.Image

			BeforeEach(func() {
				existingKpackImage = &buildv1alpha2.Image{
					ObjectMeta: metav1.ObjectMeta{
						Name:      cfBuildGUID,
						Namespace: namespaceGUID,
						Labels: map[string]string{
							controllers.BuildWorkloadLabelKey: cfBuildGUID,
						},
					},
					Spec: buildv1alpha2.ImageSpec{
						Tag: "my-tag-string",
						Builder: corev1.ObjectReference{
							Name: "my-builder",
						},
						ServiceAccountName: "my-service-account",
						Source: corev1alpha1.SourceConfig{
							Registry: &corev1alpha1.Registry{
								Image:            "not-an-image",
								ImagePullSecrets: nil,
							},
						},
					},
				}
				Expect(k8sClient.Create(context.Background(), existingKpackImage)).To(Succeed())
			})

			It("sets the status condition on BuildWorkload", func() {
				cfBuildLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				updatedBuildWorkload := new(korifiv1alpha1.BuildWorkload)
				Eventually(func(g Gomega) {
					err := k8sClient.Get(context.Background(), cfBuildLookupKey, updatedBuildWorkload)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(mustHaveCondition(g, updatedBuildWorkload.Status.Conditions, succeededConditionType).Status).To(Equal(metav1.ConditionUnknown))
				}).Should(Succeed())
			})
		})

		When("the source image pull secret doesn't exist", func() {
			var nonExistentSecret string

			BeforeEach(func() {
				nonExistentSecret = PrefixedGUID("no-such-secret")
				source.Registry.ImagePullSecrets = []corev1.LocalObjectReference{
					{Name: nonExistentSecret},
				}
			})

			It("doesn't create the kpack Image as long as the secret is missing", func() {
				Consistently(func(g Gomega) bool {
					lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
					return errors.IsNotFound(k8sClient.Get(context.Background(), lookupKey, new(buildv1alpha2.Image)))
				}).Should(BeTrue())
			})
		})

		When("reconciler name on BuildWorkload is not kpack-image-builder", func() {
			BeforeEach(func() {
				reconcilerName = "notkpackreconciler"
			})

			It("does not create a kpack image resource", func() {
				Consistently(func(g Gomega) {
					kpackImage := new(buildv1alpha2.Image)
					err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}, kpackImage)
					g.Expect(err).To(MatchError(fmt.Sprintf("images.kpack.io %q not found", cfBuildGUID)))
				}).Should(Succeed())
			})

			When("the other reconciler has partially reconciled the object and created an Image", func() {
				BeforeEach(func() {
					image := buildKpackImageObject(cfBuildGUID, namespaceGUID, source, env, services)
					Expect(k8sClient.Create(context.Background(), image)).To(Succeed())

					returnedProcessTypes := []korifiv1alpha1.ProcessType{{Type: "web", Command: "my-command"}, {Type: "db", Command: "my-command2"}}
					returnedPorts := []int32{8080, 8443}
					fakeImageProcessFetcher.Returns(returnedProcessTypes, returnedPorts, nil)

					kpackImageLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
					createdKpackImage := new(buildv1alpha2.Image)
					Eventually(func() error {
						return k8sClient.Get(context.Background(), kpackImageLookupKey, createdKpackImage)
					}).Should(Succeed())

					setKpackImageStatus(createdKpackImage, kpackReadyConditionType, metav1.ConditionTrue)
					createdKpackImage.Status.LatestImage = "some-org/my-image@sha256:some-sha"
					createdKpackImage.Status.LatestStack = "cflinuxfs3"
					Expect(k8sClient.Status().Update(context.Background(), createdKpackImage)).To(Succeed())
				})

				JustBeforeEach(func() {
					updatedBuildWorkload := new(korifiv1alpha1.BuildWorkload)
					Eventually(func(g Gomega) {
						err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}, updatedBuildWorkload)
						g.Expect(err).NotTo(HaveOccurred())
					}).Should(Succeed())

					meta.SetStatusCondition(&updatedBuildWorkload.Status.Conditions, metav1.Condition{
						Type:    korifiv1alpha1.SucceededConditionType,
						Status:  metav1.ConditionUnknown,
						Reason:  "thinking",
						Message: "thunking",
					})
					Expect(k8sClient.Status().Update(context.Background(), updatedBuildWorkload)).To(Succeed())
				})

				It("doesn't continue to reconcile the object", func() {
					updatedBuildWorkload := new(korifiv1alpha1.BuildWorkload)
					Consistently(func(g Gomega) {
						err := k8sClient.Get(context.Background(), types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}, updatedBuildWorkload)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(mustHaveCondition(g, updatedBuildWorkload.Status.Conditions, succeededConditionType).Status).To(Equal(metav1.ConditionUnknown))
						g.Expect(fakeImageProcessFetcher.CallCount()).To(BeZero())
					}).Should(Succeed())
				})

			})
		})
	})

	When("the kpack Image was already created", func() {
		var createdKpackImage *buildv1alpha2.Image

		BeforeEach(func() {
			buildWorkload = buildWorkloadObject(cfBuildGUID, namespaceGUID, source, env, services, reconcilerName)
			Expect(k8sClient.Create(context.Background(), buildWorkload)).To(Succeed())

			kpackImageLookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
			createdKpackImage = new(buildv1alpha2.Image)
			Eventually(func() error {
				return k8sClient.Get(context.Background(), kpackImageLookupKey, createdKpackImage)
			}).Should(Succeed())
		})

		When("the image build failed", func() {
			BeforeEach(func() {
				setKpackImageStatus(createdKpackImage, kpackReadyConditionType, metav1.ConditionFalse)
				Expect(k8sClient.Status().Update(context.Background(), createdKpackImage)).To(Succeed())
			})

			It("sets the Succeeded conditions to False", func() {
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				updatedWorkload := new(korifiv1alpha1.BuildWorkload)
				Eventually(func(g Gomega) {
					err := k8sClient.Get(context.Background(), lookupKey, updatedWorkload)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(mustHaveCondition(g, updatedWorkload.Status.Conditions, succeededConditionType).Status).To(Equal(metav1.ConditionFalse))
				}).Should(Succeed())
			})
		})

		When("the image build succeeded", func() {
			const (
				kpackBuildImageRef    = "some-org/my-image@sha256:some-sha"
				kpackImageLatestStack = "cflinuxfs3"
			)

			var (
				returnedProcessTypes []korifiv1alpha1.ProcessType
				returnedPorts        []int32
			)

			BeforeEach(func() {
				// Fill out fake ImageProcessFetcher
				returnedProcessTypes = []korifiv1alpha1.ProcessType{{Type: "web", Command: "my-command"}, {Type: "db", Command: "my-command2"}}
				returnedPorts = []int32{8080, 8443}
				fakeImageProcessFetcher.Returns(returnedProcessTypes, returnedPorts, nil)

				setKpackImageStatus(createdKpackImage, kpackReadyConditionType, metav1.ConditionTrue)
				createdKpackImage.Status.LatestImage = kpackBuildImageRef
				createdKpackImage.Status.LatestStack = kpackImageLatestStack
				Expect(k8sClient.Status().Update(context.Background(), createdKpackImage)).To(Succeed())
			})

			It("sets the Succeeded condition to True", func() {
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				updatedWorkload := new(korifiv1alpha1.BuildWorkload)
				Eventually(func(g Gomega) {
					err := k8sClient.Get(context.Background(), lookupKey, updatedWorkload)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(mustHaveCondition(g, updatedWorkload.Status.Conditions, succeededConditionType).Status).To(Equal(metav1.ConditionTrue))
				}).Should(Succeed())
			})

			It("sets status.droplet", func() {
				lookupKey := types.NamespacedName{Name: cfBuildGUID, Namespace: namespaceGUID}
				updatedBuildWorkload := new(korifiv1alpha1.BuildWorkload)
				Eventually(func(g Gomega) *korifiv1alpha1.BuildDropletStatus {
					err := k8sClient.Get(context.Background(), lookupKey, updatedBuildWorkload)
					g.Expect(err).NotTo(HaveOccurred())
					return updatedBuildWorkload.Status.Droplet
				}).ShouldNot(BeNil())
				Expect(fakeImageProcessFetcher.CallCount()).To(BeNumerically(">=", 1))
				Expect(updatedBuildWorkload.Status.Droplet.Registry.Image).To(Equal(kpackBuildImageRef), "droplet registry image does not match kpack image latestImage")
				Expect(updatedBuildWorkload.Status.Droplet.Stack).To(Equal(kpackImageLatestStack), "droplet stack does not match kpack image latestStack")
				Expect(updatedBuildWorkload.Status.Droplet.Registry.ImagePullSecrets).To(Equal(source.Registry.ImagePullSecrets))
				Expect(updatedBuildWorkload.Status.Droplet.ProcessTypes).To(Equal(returnedProcessTypes))
				Expect(updatedBuildWorkload.Status.Droplet.Ports).To(Equal(returnedPorts))
			})
		})
	})
})

func setKpackImageStatus(kpackImage *buildv1alpha2.Image, conditionType string, conditionStatus metav1.ConditionStatus) {
	kpackImage.Status.Conditions = append(kpackImage.Status.Conditions, corev1alpha1.Condition{
		Type:   corev1alpha1.ConditionType(conditionType),
		Status: corev1.ConditionStatus(conditionStatus),
	})
}

func buildNamespaceObject(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func buildDockerRegistrySecret(name, namespace string) *corev1.Secret {
	dockerRegistryUsername := "user"
	dockerRegistryPassword := "password"
	dockerAuth := base64.StdEncoding.EncodeToString([]byte(dockerRegistryUsername + ":" + dockerRegistryPassword))
	dockerConfigJSON := `{"auths":{"https://index.docker.io/v1/":{"username":"` + dockerRegistryUsername + `","password":"` + dockerRegistryPassword + `","auth":"` + dockerAuth + `"}}}`
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Immutable: nil,
		Data:      nil,
		StringData: map[string]string{
			".dockerconfigjson": dockerConfigJSON,
		},
		Type: "kubernetes.io/dockerconfigjson",
	}
}

func buildServiceAccount(name, namespace, imagePullSecretName string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Secrets:          []corev1.ObjectReference{{Name: imagePullSecretName}},
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: imagePullSecretName}},
	}
}

func PrefixedGUID(prefix string) string {
	return prefix + "-" + uuid.NewString()[:8]
}

func buildWorkloadObject(cfBuildGUID string, namespace string, source korifiv1alpha1.PackageSource, env []corev1.EnvVar, services []corev1.ObjectReference, reconcilerName string) *korifiv1alpha1.BuildWorkload {
	return &korifiv1alpha1.BuildWorkload{
		ObjectMeta: metav1.ObjectMeta{

			Name:      cfBuildGUID,
			Namespace: namespace,
		},
		Spec: korifiv1alpha1.BuildWorkloadSpec{
			BuildRef: corev1.LocalObjectReference{
				Name: cfBuildGUID,
			},
			Source:         source,
			Env:            env,
			Services:       services,
			ReconcilerName: reconcilerName,
		},
	}
}

func mustHaveCondition(g Gomega, conditions []metav1.Condition, conditionType string) *metav1.Condition {
	foundCondition := meta.FindStatusCondition(conditions, conditionType)
	g.ExpectWithOffset(1, foundCondition).NotTo(BeNil())
	return foundCondition
}

func buildKpackImageObject(name string, namespace string, source korifiv1alpha1.PackageSource, env []corev1.EnvVar, services []corev1.ObjectReference) *buildv1alpha2.Image {
	return &buildv1alpha2.Image{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				controllers.BuildWorkloadLabelKey: name,
			},
		},
		Spec: buildv1alpha2.ImageSpec{
			Tag: "kpack-image-tag",
			Builder: corev1.ObjectReference{
				Kind:       "ClusterBuilder",
				Name:       "default",
				APIVersion: "kpack.io/v1alpha2",
			},
			ServiceAccountName: "kpack-service-account",
			Source: corev1alpha1.SourceConfig{
				Registry: &corev1alpha1.Registry{
					Image:            source.Registry.Image,
					ImagePullSecrets: source.Registry.ImagePullSecrets,
				},
			},
			Build: &buildv1alpha2.ImageBuild{
				Services: services,
				Env:      env,
			},
		},
	}
}
