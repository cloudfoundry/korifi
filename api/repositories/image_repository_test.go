package repositories_test

import (
	"bytes"
	"context"
	"errors"
	"io"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/repositories"
	"code.cloudfoundry.org/korifi/api/repositories/fake"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8sclient "k8s.io/client-go/kubernetes"
)

var _ = Describe("ImageRepository", func() {
	var (
		imagePusher         *fake.ImagePusher
		privilegedK8sClient k8sclient.Interface
		imageSource         io.Reader
		imageRepo           *repositories.ImageRepository
		imageName           string
		imageRef            string
		tags                []string
		uploadErr           error
		org                 *korifiv1alpha1.CFOrg
		space               *korifiv1alpha1.CFSpace
	)

	BeforeEach(func() {
		imageName = "my-image"
		imagePusher = new(fake.ImagePusher)
		imagePusher.PushReturns("my-pushed-image", nil)

		imageSource = bytes.NewBufferString("")

		var err error
		privilegedK8sClient, err = k8sclient.NewForConfig(k8sConfig)
		Expect(err).NotTo(HaveOccurred())

		org = createOrgWithCleanup(ctx, prefixedGUID("org"))
		space = createSpaceWithCleanup(ctx, org.Name, prefixedGUID("space"))

		tags = []string{"foo", "bar"}

		imageRepo = repositories.NewImageRepository(
			privilegedK8sClient,
			userClientFactory,
			imagePusher,
			"push-secret-name",
			rootNamespace,
		)
	})

	JustBeforeEach(func() {
		imageRef, uploadErr = imageRepo.UploadSourceImage(context.Background(), authInfo, imageName, imageSource, space.Name, tags...)
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
			_, creds, actualRef, zipReader, actualTags := imagePusher.PushArgsForCall(0)
			Expect(creds.Namespace).To(Equal(rootNamespace))
			Expect(creds.SecretName).To(Equal("push-secret-name"))
			Expect(actualRef).To(Equal("my-image"))
			Expect(zipReader).To(Equal(imageSource))
			Expect(actualTags).To(Equal(tags))
		})

		When("the image name is invalid", func() {
			BeforeEach(func() {
				imageName = "invAlid-image"
			})

			It("fails with an easy to understand unprocessible entity error ", func() {
				var apiError apierrors.UnprocessableEntityError
				Expect(errors.As(uploadErr, &apiError)).To(BeTrue())
				Expect(apiError.Detail()).To(Equal(`invalid image ref: "invAlid-image"`))
			})
		})

		When("pushing the image fails", func() {
			BeforeEach(func() {
				imagePusher.PushReturns("", errors.New("push-error"))
			})

			It("fails with a blobstore unavailable error", func() {
				Expect(uploadErr).To(MatchError(ContainSubstring("push-error")))
				var apiError apierrors.BlobstoreUnavailableError
				Expect(errors.As(uploadErr, &apiError)).To(BeTrue())
				Expect(apiError.Detail()).To(Equal("Error uploading source package to the container registry"))
			})
		})
	})
})
