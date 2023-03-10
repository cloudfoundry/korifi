package image

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/buildpacks/pack/pkg/archive"
	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/authn/k8schain"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
)

type Client struct {
	k8sClient  kubernetes.Interface
	namespace  string
	secretName string
	logger     logr.Logger
}

func NewClient(k8sClient kubernetes.Interface, namespace, secretName string) Client {
	return Client{
		k8sClient:  k8sClient,
		namespace:  namespace,
		secretName: secretName,
		logger:     ctrl.Log.WithName("image.client"),
	}
}

func (c Client) Push(ctx context.Context, repoRef string, zipReader io.Reader, tags ...string) (string, error) {
	tmpFile, err := os.CreateTemp(os.TempDir(), "sourceimg-%s")
	if err != nil {
		return "", fmt.Errorf("failed to create a temp file for image: %w", err)
	}
	defer tmpFile.Close()

	if _, err = io.Copy(tmpFile, zipReader); err != nil {
		return "", fmt.Errorf("failed to copy image source into temp file '%s' %w", tmpFile.Name(), err)
	}

	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return archive.ReadZipAsTar(tmpFile.Name(), "/", 0, 0, -1, true, nil), nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to create a layer out of '%s': %w", tmpFile.Name(), err)
	}

	image, err := mutate.AppendLayers(empty.Image, layer)
	if err != nil {
		return "", fmt.Errorf("failed to append layer: %w", err)
	}

	ref, err := name.ParseReference(repoRef)
	if err != nil {
		return "", fmt.Errorf("error parsing repository reference %s: %w", repoRef, err)
	}

	authOpt, err := c.authOpt(ctx)
	if err != nil {
		return "", fmt.Errorf("error creating keychain: %w", err)
	}

	if err = remote.Write(ref, image, authOpt); err != nil {
		return "", fmt.Errorf("failed to upload image: %w", err)
	}

	for _, tag := range tags {
		err = remote.Tag(ref.Context().Tag(tag), image, authOpt)
		if err != nil {
			return "", fmt.Errorf("failed to tag image: %w", err)
		}
	}

	imgDigest, err := image.Digest()
	if err != nil {
		return "", fmt.Errorf("failed to get image digest: %w", err)
	}

	refWithDigest, err := name.NewDigest(fmt.Sprintf("%s@%s", ref.Context().Name(), imgDigest.String()))
	if err != nil {
		return "", fmt.Errorf("failed to create digest: %w", err)
	}

	return refWithDigest.Name(), nil
}

func (c Client) Labels(ctx context.Context, imageRef string) (map[string]string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("error parsing repository reference %s: %w", imageRef, err)
	}

	authOpt, err := c.authOpt(ctx)
	if err != nil {
		return nil, fmt.Errorf("error creating keychain: %w", err)
	}

	img, err := remote.Image(ref, authOpt)
	if err != nil {
		return nil, fmt.Errorf("failed to get image: %w", err)
	}

	cfgFile, err := img.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("error getting image config file: %w", err)
	}

	return cfgFile.Config.Labels, nil
}

func (c Client) Delete(ctx context.Context, imageRef string) error {
	c.logger.V(1).Info("deleting", "ref", imageRef)
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return err
	}

	authOpt, err := c.authOpt(ctx)
	if err != nil {
		return fmt.Errorf("error creating keychain: %w", err)
	}

	tags, err := remote.List(ref.Context(), authOpt)
	if err != nil {
		c.logger.V(1).Info("failed to list tags - skipping tag deletion", "reason", err)
	}

	for _, tag := range tags {
		var tagRef name.Reference
		tagRef, err = name.ParseReference(ref.Context().String() + ":" + tag)
		if err != nil {
			return fmt.Errorf("couldn't create a tag ref: %w", err)
		}
		var descriptor *remote.Descriptor
		descriptor, err = remote.Get(tagRef, authOpt)
		if err != nil {
			c.logger.V(1).Info("failed get tag - continuing", "reason", err)
			continue
		}

		if descriptor.Digest.String() == ref.Identifier() {
			c.logger.V(1).Info("deleting tag", "tag", tag)
			err = remote.Delete(descriptor.Ref, authOpt)
			if err != nil {
				c.logger.V(1).Info("failed to delete tag", "reason", err)
			}
		}
	}

	err = remote.Delete(ref, authOpt)
	if err != nil {
		if structuredErr, ok := err.(*transport.Error); ok && structuredErr.StatusCode == http.StatusNotFound {
			c.logger.V(1).Info("manifest disappeared - continuing", "reason", err)
			return nil
		}
	}

	return err
}

func (c Client) authOpt(ctx context.Context) (remote.Option, error) {
	var keychain authn.Keychain
	var err error

	if c.secretName == "" {
		keychain, err = k8schain.NewNoClient(ctx)
	} else {
		keychain, err = k8schain.New(ctx, c.k8sClient, k8schain.Options{
			Namespace:        c.namespace,
			ImagePullSecrets: []string{c.secretName},
		})
	}
	if err != nil {
		return nil, err
	}

	return remote.WithAuthFromKeychain(keychain), nil
}
