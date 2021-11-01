package repositories

import (
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"os"
	"path"

	"github.com/buildpacks/pack/pkg/archive"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

func UploadSourceImage(imageRef string, packageSrcFile multipart.File, credentialOption remote.Option) (imageRefWithDigest string, err error) {
	tmpFile, err := ioutil.TempFile(os.TempDir(), fmt.Sprintf("source-%s", path.Base(imageRef)))
	if err != nil {
		return "", fmt.Errorf("error from ioutil.TempFile: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err = io.Copy(tmpFile, packageSrcFile); err != nil {
		return "", fmt.Errorf("error from io.Copy: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("error from ioutil.TempFile: %w", err)
	}

	image, err := random.Image(0, 0)
	if err != nil {
		return "", fmt.Errorf("error from random.Image: %w", err)
	}

	noopFilter := func(string) bool { return true }
	layer, err := tarball.LayerFromReader(archive.ReadZipAsTar(tmpFile.Name(), "/", 0, 0, -1, true, noopFilter))
	if err != nil {
		return "", fmt.Errorf("error from tarball.LayerFromReader: %w", err)
	}

	image, err = mutate.AppendLayers(image, layer)
	if err != nil {
		return "", fmt.Errorf("error from mutate.AppendLayers: %w", err)
	}

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("error from name.ParseReference: %w", err)
	}

	err = remote.Write(ref, image, credentialOption)
	if err != nil {
		return "", fmt.Errorf("error from remote.Write: %w", err)
	}

	imgDigest, err := image.Digest()
	if err != nil {
		return "", fmt.Errorf("error from image.Digest: %w", err)
	}

	refWithDigest, err := name.NewDigest(fmt.Sprintf("%s@%s", ref.Context().Name(), imgDigest.String()))
	if err != nil {
		return "", fmt.Errorf("error from name.NewDigest: %w", err)
	}

	return refWithDigest.Name(), nil
}
