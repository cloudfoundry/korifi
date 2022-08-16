package repositories_test

import (
	"bytes"
	"context"
	"errors"
	"io"

	"code.cloudfoundry.org/korifi/api/apierrors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "k8s.io/client-go/kubernetes"
)

var _ = Describe("ImageRepository", func() {
	var (
		registrySecretName  string
		registryCAPath      string
		imageBuilder        *fake.ImageBuilder
		imagePusher         *fake.ImagePusher
		image               v1.Image
		privilegedK8sClient k8sclient.Interface

		imageSource io.Reader

		imageRepo *repositories.ImageRepository

		imageRef  string
		uploadErr error
		org       *korifiv1alpha1.CFOrg
		space     *korifiv1alpha1.CFSpace
	)

	BeforeEach(func() {
		var err error
		imageBuilder = new(fake.ImageBuilder)
		image, err = random.Image(0, 0)
		Expect(err).NotTo(HaveOccurred())
		imageBuilder.BuildReturns(image, nil)

		imagePusher = new(fake.ImagePusher)
		imagePusher.PushReturns("my-pushed-image", nil)

		imageSource = bytes.NewBufferString("")

		privilegedK8sClient, err = k8sclient.NewForConfig(k8sConfig)
		Expect(err).NotTo(HaveOccurred())

		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))

		_, err = privilegedK8sClient.
			CoreV1().
			ServiceAccounts(rootNamespace).
			Create(
				context.Background(),
				&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "default",
						Namespace: rootNamespace,
					},
				},
				metav1.CreateOptions{},
			)
		Expect(err).NotTo(HaveOccurred())

		registrySecretName = prefixedGUID("registry-secret")
		_, err = privilegedK8sClient.CoreV1().
			Secrets(rootNamespace).
			Create(
				context.Background(),
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      registrySecretName,
						Namespace: rootNamespace,
					},
					StringData: map[string]string{
						corev1.DockerConfigJsonKey: "{}",
					},
					Type: corev1.SecretTypeDockerConfigJson,
				},
				metav1.CreateOptions{},
			)
		Expect(err).NotTo(HaveOccurred())

		registryCAPath = ""

		imageRepo = repositories.NewImageRepository(
			privilegedK8sClient,
			userClientFactory,
			rootNamespace,
			registrySecretName,
			registryCAPath,
			imageBuilder,
			imagePusher,
		)
	})

	JustBeforeEach(func() {
		imageRef, uploadErr = imageRepo.UploadSourceImage(context.Background(), authInfo, "my-image", imageSource, space.Name)
	})

	It("fails with unauthorized error without a valid role in the space", func() {
		Expect(uploadErr).To(BeAssignableToTypeOf(apierrors.ForbiddenError{}))
	})

	When("user has role SpaceDeveloper", func() {
		BeforeEach(func() {
			createRoleBinding(context.Background(), userName, spaceDeveloperRole.Name, space.Name)
		})

		It("succeeds", func() {
			Expect(uploadErr).NotTo(HaveOccurred())
			Expect(imageRef).To(Equal("my-pushed-image"))
		})

		It("uploads the image to the registry", func() {
			Expect(imagePusher.PushCallCount()).To(Equal(1))
			actualRef, actualImage, credentials, _ := imagePusher.PushArgsForCall(0)
			Expect(actualRef).To(Equal("my-image"))
			Expect(actualImage).To(Equal(image))
			Expect(credentials).NotTo(BeNil())
		})

		When("building the image fails", func() {
			BeforeEach(func() {
				imageBuilder.BuildReturns(nil, errors.New("build-error"))
			})

			It("errors", func() {
				Expect(uploadErr).To(MatchError(ContainSubstring("build-error")))
			})
		})

		When("pushing the image fails", func() {
			BeforeEach(func() {
				imagePusher.PushReturns("", errors.New("push-error"))
			})

			It("errors", func() {
				Expect(uploadErr).To(MatchError(ContainSubstring("push-error")))
			})
		})

		When("getting the registry credentials fails", func() {
			BeforeEach(func() {
				Expect(privilegedK8sClient.CoreV1().
					Secrets(rootNamespace).
					Delete(context.Background(), registrySecretName, metav1.DeleteOptions{})).To(Succeed())
			})

			It("errors", func() {
				Expect(uploadErr).To(MatchError(ContainSubstring("getting push credentials")))
			})
		})
	})
})
