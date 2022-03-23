package registry_test

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"os"

	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/registry"
	"code.cloudfoundry.org/cf-k8s-controllers/api/repositories/registry/fake"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

//counterfeiter:generate -o fake -fake-name Reader io.Reader

var _ = Describe("ImageBuilder", func() {
	var (
		imageBuilder     *registry.ImageBuilder
		imageLayerReader *fake.Reader

		builtImage v1.Image
		buildErr   error
	)

	BeforeEach(func() {
		imageLayerReader = new(fake.Reader)
		imageLayerReader.ReadStub = func(b []byte) (int, error) {
			bs, err := os.ReadFile("fixtures/layer.zip")
			if err != nil {
				return 0, err
			}
			n := copy(b, bs)
			return n, io.EOF
		}
		imageBuilder = registry.NewImageBuilder()
	})

	JustBeforeEach(func() {
		builtImage, buildErr = imageBuilder.Build(context.Background(), imageLayerReader)
	})

	It("builds an an image with a layer containing the source file", func() {
		imgLayers := getImageLayers(builtImage)
		Expect(imgLayers).To(HaveLen(1))

		layerContent, err := imgLayers[0].Uncompressed()
		Expect(err).NotTo(HaveOccurred())

		layerReader := tar.NewReader(layerContent)
		header, err := layerReader.Next()
		Expect(err).NotTo(HaveOccurred())
		Expect(header.FileInfo().Name()).To(Equal("foo"))
	})

	When("reading from the source reader fails", func() {
		BeforeEach(func() {
			imageLayerReader.ReadReturns(0, errors.New("error-from-reader"))
		})

		It("returns the error", func() {
			Expect(buildErr).To(MatchError(ContainSubstring("error-from-reader")))
		})
	})

	When("the source file is not a zip", func() {
		BeforeEach(func() {
			imageLayerReader.ReadStub = func(b []byte) (int, error) {
				copy(b, []byte("not a zip"))
				return 9, io.EOF
			}
		})

		It("returns an error", func() {
			Expect(buildErr).To(MatchError(ContainSubstring("not a valid zip file")))
		})
	})
})

func getImageLayers(image v1.Image) []v1.Layer {
	imgLayers, err := image.Layers()
	Expect(err).NotTo(HaveOccurred())
	return imgLayers
}
