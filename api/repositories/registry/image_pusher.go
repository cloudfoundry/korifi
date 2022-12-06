package registry

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

//counterfeiter:generate -o fake -fake-name ImageWriter . ImageWriter

type (
	ImageWriter func(ref name.Reference, img v1.Image, options ...remote.Option) error
)

type ImagePusher struct {
	imageWriter ImageWriter
}

func NewImagePusher(imageWriter ImageWriter) *ImagePusher {
	return &ImagePusher{
		imageWriter: imageWriter,
	}
}

func (r *ImagePusher) Push(repoRef string, image v1.Image, credentials remote.Option) (string, error) {
	ref, err := r.uploadImage(image, repoRef, credentials)
	if err != nil {
		return "", err
	}

	return buildRefWithDigest(image, ref)
}

func (r *ImagePusher) uploadImage(image v1.Image, repoRef string, credentials remote.Option) (name.Reference, error) {
	ref, err := name.ParseReference(repoRef)
	if err != nil {
		return nil, fmt.Errorf("error parsing repository reference %s: %w", repoRef, err)
	}

	err = r.imageWriter(ref, image, credentials)
	if err != nil {
		return nil, fmt.Errorf("failed to upload image: %w", err)
	}

	return ref, nil
}

func buildRefWithDigest(image v1.Image, ref name.Reference) (string, error) {
	imgDigest, err := image.Digest()
	if err != nil {
		return "", fmt.Errorf("failed to get image digest: %w", err)
	}

	refWithDigest, err := name.NewDigest(fmt.Sprintf("%s@%s", ref.Context().Name(), imgDigest.String()))
	if err != nil {
		return "", fmt.Errorf("failed to create diges: %w", err)
	}

	return refWithDigest.Name(), nil
}
