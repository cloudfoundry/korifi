package oci

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"

	"github.com/distribution/distribution/v3/configuration"
	dcontext "github.com/distribution/distribution/v3/context"
	_ "github.com/distribution/distribution/v3/registry/auth/htpasswd"
	"github.com/distribution/distribution/v3/registry/handlers"
	_ "github.com/distribution/distribution/v3/registry/storage/driver/inmemory"
	"github.com/foomo/htpasswd"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
	"github.com/sirupsen/logrus"
)

func init() {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	logger.SetOutput(GinkgoWriter)
	dcontext.SetDefaultLogger(logrus.NewEntry(logger))
}

type Registry struct {
	server   *httptest.Server
	username string
	password string
}

func (r *Registry) URL() string {
	return r.server.URL
}

func (r *Registry) ImageRef(relativeImageRef string) string {
	serverURL, err := url.Parse(r.URL())
	Expect(err).NotTo(HaveOccurred())
	return fmt.Sprintf("%s/%s", serverURL.Host, relativeImageRef)
}

func (r *Registry) PushImage(repoRef string, imageConfig *v1.ConfigFile) {
	image, err := mutate.ConfigFile(empty.Image, imageConfig)
	Expect(err).NotTo(HaveOccurred())

	ref, err := name.ParseReference(repoRef)
	Expect(err).NotTo(HaveOccurred())

	pushOpts := []remote.Option{}
	if r.username != "" && r.password != "" {
		pushOpts = append(pushOpts, remote.WithAuth(&authn.Basic{
			Username: r.username,
			Password: r.password,
		}))
	}
	Expect(remote.Write(ref, image, pushOpts...)).To(Succeed())
}

func NewContainerRegistry(username, password string) *Registry {
	htpasswdFile := generateHtpasswdFile(username, password)

	registry := &Registry{
		server: httptest.NewServer(handlers.NewApp(context.Background(), &configuration.Configuration{
			Auth: configuration.Auth{
				"htpasswd": configuration.Parameters{
					"realm": "Registry Realm",
					"path":  htpasswdFile,
				},
			},
			Storage: configuration.Storage{
				"inmemory": configuration.Parameters{},
				"delete":   configuration.Parameters{"enabled": true},
			},
			Loglevel: "debug",
		})),
		username: username,
		password: password,
	}

	DeferCleanup(func() {
		registry.server.Close()
		Expect(os.RemoveAll(htpasswdFile)).To(Succeed())
	})

	return registry
}

func NewNoAuthContainerRegistry() *Registry {
	registry := &Registry{
		server: httptest.NewServer(handlers.NewApp(context.Background(), &configuration.Configuration{
			Storage: configuration.Storage{
				"inmemory": configuration.Parameters{},
				"delete":   configuration.Parameters{"enabled": true},
			},
			Loglevel: "debug",
		})),
	}

	DeferCleanup(func() {
		registry.server.Close()
	})

	return registry
}

func generateHtpasswdFile(username, password string) string {
	htpasswdFile, err := os.CreateTemp("", "")
	Expect(err).NotTo(HaveOccurred())

	Expect(htpasswd.SetPassword(htpasswdFile.Name(), username, password, htpasswd.HashBCrypt)).To(Succeed())

	return htpasswdFile.Name()
}
