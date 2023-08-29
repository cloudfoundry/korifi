package image

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

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
	"k8s.io/utils/net"
	ctrl "sigs.k8s.io/controller-runtime"
)

type Client struct {
	k8sClient kubernetes.Interface
	logger    logr.Logger
}

type Creds struct {
	Namespace string
	// At most one of SecretNames and ServiceAccountName should be set.
	// If both unset, the fallback auth approach will be used.
	SecretNames        []string
	ServiceAccountName string
}

type Config struct {
	Labels       map[string]string
	ExposedPorts []int32
}

func NewClient(k8sClient kubernetes.Interface) Client {
	return Client{
		k8sClient: k8sClient,
		logger:    ctrl.Log.WithName("image.client"),
	}
}

func (c Client) Push(ctx context.Context, creds Creds, repoRef string, zipReader io.Reader, tags ...string) (string, error) {
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

	authOpt, err := c.authOpt(ctx, creds)
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

func (c Client) Config(ctx context.Context, creds Creds, imageRef string) (Config, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return Config{}, fmt.Errorf("error parsing repository reference %s: %w", imageRef, err)
	}

	authOpt, err := c.authOpt(ctx, creds)
	if err != nil {
		return Config{}, fmt.Errorf("error creating keychain: %w", err)
	}

	img, err := remote.Image(ref, authOpt)
	if err != nil {
		return Config{}, fmt.Errorf("failed to get image: %w", err)
	}

	cfgFile, err := img.ConfigFile()
	if err != nil {
		return Config{}, fmt.Errorf("error getting image config file: %w", err)
	}

	ports := []int32{}
	for _, p := range parseExposedPorts(cfgFile.Config.ExposedPorts) {
		parsed, err := net.ParsePort(p, false)
		if err != nil {
			return Config{}, fmt.Errorf("error getting exposed ports: %w", err)
		}
		ports = append(ports, int32(parsed))
	}

	return Config{
		Labels:       cfgFile.Config.Labels,
		ExposedPorts: ports,
	}, nil
}

func parseExposedPorts(ports map[string]struct{}) []string {
	result := []string{}
	for p := range ports {
		result = append(result, strings.Split(p, "/")[0])
	}

	return result
}

func (c Client) Delete(ctx context.Context, creds Creds, imageRef string, tagsToDelete ...string) error {
	c.logger.V(1).Info("deleting", "ref", imageRef)
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return err
	}

	authOpt, err := c.authOpt(ctx, creds)
	if err != nil {
		return fmt.Errorf("error creating keychain: %w", err)
	}

	allTagSet, err := c.getTagSet(ref, authOpt)
	if err != nil {
		return fmt.Errorf("failed to list tags: %w", err)
	}

	for _, tag := range tagsToDelete {
		if !allTagSet[tag] {
			c.logger.Info("tag not found for this ref", "tag", tag, "ref", imageRef)
			continue
		}

		if err = c.deleteTag(ref, tag, authOpt); err != nil {
			c.logger.Info("failed to delete tag", "reason", err)
			continue
		}
		delete(allTagSet, tag)
	}

	// The latest tag is set automatically by registries. If it is the only
	// remaining tag, remove it to prevent digest deletion errors
	latestTag := "latest"
	if len(allTagSet) == 1 && allTagSet[latestTag] {
		if err = c.deleteTag(ref, latestTag, authOpt); err != nil {
			c.logger.Info("failed to delete tag", "reason", err)
		} else {
			delete(allTagSet, latestTag)
		}
	}

	if len(allTagSet) == 0 {
		err = remote.Delete(ref, authOpt)
		if err != nil {
			if structuredErr, ok := err.(*transport.Error); ok && structuredErr.StatusCode == http.StatusNotFound {
				c.logger.V(1).Info("manifest disappeared - continuing", "reason", err)
				return nil
			}
		}
	}

	return err
}

func (c Client) getTagSet(ref name.Reference, authOpt remote.Option) (map[string]bool, error) {
	allTags, err := remote.List(ref.Context(), authOpt)
	if err != nil {
		c.logger.V(1).Info("failed to list tags - skipping tag deletion", "reason", err)
		return nil, err
	}

	allTagSet := map[string]bool{}
	for _, t := range allTags {
		var tagRef name.Reference
		tagRef, err = name.ParseReference(ref.Context().String() + ":" + t)
		if err != nil {
			return nil, fmt.Errorf("couldn't create a tag ref: %w", err)
		}

		var descriptor *remote.Descriptor
		descriptor, err = remote.Get(tagRef, authOpt)
		if err != nil {
			return nil, fmt.Errorf("couldn't get tag: %w", err)
		}

		if descriptor.Digest.String() == ref.Identifier() {
			allTagSet[t] = true
		}
	}

	return allTagSet, nil
}

func (c Client) deleteTag(ref name.Reference, tag string, authOpt remote.Option) error {
	tagRef, err := name.ParseReference(ref.Context().String() + ":" + tag)
	if err != nil {
		return fmt.Errorf("couldn't create a tag ref: %w", err)
	}
	var descriptor *remote.Descriptor
	descriptor, err = remote.Get(tagRef, authOpt)
	if err != nil {
		c.logger.V(1).Info("failed get tag - continuing", "reason", err)
		return nil
	}

	if descriptor.Digest.String() == ref.Identifier() {
		c.logger.V(1).Info("deleting tag", "tag", tag)
		err = remote.Delete(tagRef, authOpt)
		if err != nil {
			c.logger.V(1).Info("failed to delete tag", "reason", err)
		}
	}

	return nil
}

func (c Client) authOpt(ctx context.Context, creds Creds) (remote.Option, error) {
	var keychain authn.Keychain
	var err error

	if len(creds.SecretNames) > 0 {
		keychain, err = k8schain.New(ctx, c.k8sClient, k8schain.Options{
			Namespace:        creds.Namespace,
			ImagePullSecrets: creds.SecretNames,
		})
	} else if creds.ServiceAccountName != "" {
		keychain, err = k8schain.New(ctx, c.k8sClient, k8schain.Options{
			Namespace:          creds.Namespace,
			ServiceAccountName: creds.ServiceAccountName,
		})
	} else {
		keychain, err = k8schain.NewNoClient(ctx)
	}
	if err != nil {
		return nil, err
	}

	return remote.WithAuthFromKeychain(keychain), nil
}
