package registry_test

import (
	"errors"
	"fmt"

	"code.cloudfoundry.org/korifi/api/repositories/registry"
	"code.cloudfoundry.org/korifi/api/repositories/registry/fake"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ImagePusher", func() {
	var (
		imageWriter *fake.ImageWriter
		credentials remote.Option
		imageRef    string
		image       v1.Image

		imagePusher    *registry.ImagePusher
		pushedImageRef string
		pushErr        error
	)

	BeforeEach(func() {
		imageRef = "my-image-ref"

		credentials = remote.WithAuth(nil)

		imageWriter = new(fake.ImageWriter)

		var err error
		image, err = random.Image(0, 0)
		Expect(err).NotTo(HaveOccurred())

		imagePusher = registry.NewImagePusher(imageWriter.Spy)
	})

	JustBeforeEach(func() {
		pushedImageRef, pushErr = imagePusher.Push(imageRef, image, credentials)
	})

	It("pushes the image to the registry", func() {
		Expect(pushErr).NotTo(HaveOccurred())
		Expect(imageWriter.CallCount()).To(Equal(1))

		actualRef, actualImage, actualOptions := imageWriter.ArgsForCall(0)
		Expect(actualRef.Name()).To(Equal("index.docker.io/library/my-image-ref:latest"))
		Expect(actualImage).To(Equal(image))
		Expect(actualOptions).To(HaveLen(1))
		Expect(fmt.Sprintf("%#v", actualOptions[0])).To(Equal(fmt.Sprintf("%#v", credentials)))

		Expect(pushedImageRef).To(HavePrefix("index.docker.io/library/my-image-ref"))
	})

	When("the image reference is invalid", func() {
		BeforeEach(func() {
			imageRef = ""
		})

		It("returns an error", func() {
			Expect(pushErr).To(MatchError(ContainSubstring("could not parse reference")))
		})

		It("does not upload anything to the registry", func() {
			Expect(imageWriter.CallCount()).To(BeZero())
		})
	})

	When("writing the image to the registry fails", func() {
		BeforeEach(func() {
			imageWriter.Returns(errors.New("writing-the-image-failed"))
		})

		It("returns the error", func() {
			Expect(pushErr).To(MatchError(ContainSubstring("writing-the-image-failed")))
		})
	})
})
