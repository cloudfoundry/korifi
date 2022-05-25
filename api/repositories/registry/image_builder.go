package registry

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/buildpacks/pack/pkg/archive"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

type ImageBuilder struct{}

func NewImageBuilder() *ImageBuilder {
	return &ImageBuilder{}
}

func (r *ImageBuilder) Build(ctx context.Context, srcReader io.Reader) (v1.Image, error) {
	tempFile, err := copyIntoTempFile(srcReader)
	if err != nil {
		return nil, err
	}

	return createImage(tempFile)
}

func copyIntoTempFile(srcReader io.Reader) (string, error) {
	tmpFile, err := ioutil.TempFile(os.TempDir(), "sourceimg-%s")
	if err != nil {
		return "", fmt.Errorf("failed to create a temp file for image: %w", err)
	}
	defer tmpFile.Close()

	if _, err = io.Copy(tmpFile, srcReader); err != nil {
		return "", fmt.Errorf("failed to copy image source into temp file '%s' %w", tmpFile.Name(), err)
	}
	return tmpFile.Name(), nil
}

func createImage(srcFile string) (v1.Image, error) {
	image, err := random.Image(0, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new image: %w", err)
	}

	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return archive.ReadZipAsTar(srcFile, "/", 0, 0, -1, true, nil), nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create a layer out of '%s': %w", srcFile, err)
	}

	image, err = mutate.AppendLayers(image, layer)
	if err != nil {
		return nil, fmt.Errorf("failed to append layer: %w", err)
	}

	return image, nil
}
