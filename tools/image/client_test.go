package image_test

import (
	"os"
	"strings"

	"code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tools/image"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Client", func() {
	var (
		zipFile *os.File
		testErr error
		pushRef string
		imgRef  string
		creds   image.Creds
	)

	BeforeEach(func() {
		testErr = nil
		var err error
		zipFile, err = os.Open("fixtures/layer.zip")
		Expect(err).NotTo(HaveOccurred())

		pushRef = strings.Replace(authRegistryServer.URL+"/foo/bar", "http://", "", 1)

		imgClient = image.NewClient(k8sClientset)
		creds = image.Creds{
			Namespace:  "default",
			SecretName: secretName,
		}
	})

	Describe("Push", func() {
		JustBeforeEach(func() {
			imgRef, testErr = imgClient.Push(ctx, creds, pushRef, zipFile, "jim")
		})

		It("pushes a zip archive as an image to the registry", func() {
			Expect(testErr).NotTo(HaveOccurred())
			Expect(imgRef).To(HavePrefix(pushRef))

			_, err := imgClient.Config(ctx, creds, imgRef)
			Expect(err).NotTo(HaveOccurred())

			_, err = imgClient.Config(ctx, creds, pushRef+":jim")
			Expect(err).NotTo(HaveOccurred())
		})

		When("pushRef is invalid", func() {
			BeforeEach(func() {
				pushRef += ":bar:baz"
			})

			It("fails", func() {
				Expect(testErr).To(MatchError(ContainSubstring("error parsing repository reference")))
			})
		})

		When("zip input is not valid", func() {
			BeforeEach(func() {
				var err error
				zipFile, err = os.Open("fixtures/not.a.zip")
				Expect(err).NotTo(HaveOccurred())
			})

			It("fails", func() {
				Expect(testErr).To(MatchError(ContainSubstring("not a valid zip file")))
			})
		})

		When("the secret doesn't exist", func() {
			BeforeEach(func() {
				creds.SecretName = "not-a-secret"
			})

			It("fails to authenticate", func() {
				Expect(testErr).To(MatchError(ContainSubstring("Unauthorized")))
			})
		})

		When("using a service account for secrets", func() {
			BeforeEach(func() {
				creds.SecretName = ""
				creds.ServiceAccountName = serviceAccountName
			})

			It("pushes a zip archive as an image to the registry", func() {
				Expect(testErr).NotTo(HaveOccurred())
				Expect(imgRef).To(HavePrefix(pushRef))

				_, err := imgClient.Config(ctx, creds, imgRef)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("simulating ECR", func() {
			BeforeEach(func() {
				pushRef = strings.Replace(noAuthRegistryServer.URL+"/foo/bar", "http://", "", 1)
			})

			When("the secret name is empty", func() {
				BeforeEach(func() {
					imgClient = image.NewClient(k8sClientset)
				})

				It("succeeds", func() {
					Expect(testErr).NotTo(HaveOccurred())

					Expect(imgRef).To(HavePrefix(pushRef))
				})
			})
		})
	})

	Describe("Config", func() {
		var config image.Config

		BeforeEach(func() {
			pushRef += "/with/labels"
			pushImgWithLabelsAndPorts(pushRef, map[string]string{"foo": "bar"}, []string{"123", "456"})
		})

		JustBeforeEach(func() {
			config, testErr = imgClient.Config(ctx, creds, pushRef)
		})

		It("fetches the image config", func() {
			Expect(config.Labels).To(Equal(map[string]string{"foo": "bar"}))
			Expect(config.ExposedPorts).To(Equal([]int32{123, 456}))
		})

		When("the ref is invalid", func() {
			BeforeEach(func() {
				pushRef += "::ads"
			})

			It("fails", func() {
				Expect(testErr).To(MatchError(ContainSubstring("error parsing repository reference")))
			})
		})

		When("the secret doesn't exist", func() {
			BeforeEach(func() {
				creds.SecretName = "not-a-secret"
			})

			It("fails to authenticate", func() {
				Expect(testErr).To(MatchError(ContainSubstring("UNAUTHORIZED")))
			})
		})

		When("using a service account for secrets", func() {
			BeforeEach(func() {
				creds.SecretName = ""
				creds.ServiceAccountName = serviceAccountName
			})

			It("fetches the image labels", func() {
				Expect(config.Labels).To(Equal(map[string]string{"foo": "bar"}))
				Expect(config.ExposedPorts).To(Equal([]int32{123, 456}))
			})
		})

		When("simulating ECR", func() {
			BeforeEach(func() {
				pushRef = strings.Replace(noAuthRegistryServer.URL+"/foo/bar/with/labels", "http://", "", 1)
				pushImgWithLabelsAndPorts(pushRef, map[string]string{"foo": "bar"}, []string{"123", "456"})
			})

			When("the secret name is empty", func() {
				BeforeEach(func() {
					imgClient = image.NewClient(k8sClientset)
				})

				It("succeeds", func() {
					Expect(testErr).NotTo(HaveOccurred())
					Expect(config.Labels).To(Equal(map[string]string{"foo": "bar"}))
					Expect(config.ExposedPorts).To(Equal([]int32{123, 456}))
				})
			})
		})
	})

	Describe("Delete", func() {
		BeforeEach(func() {
			var err error
			imgRef, err = imgClient.Push(ctx, creds, pushRef, zipFile, "jim", "bob")
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			testErr = imgClient.Delete(ctx, creds, imgRef)
		})

		It("deletes an image", func() {
			Expect(testErr).NotTo(HaveOccurred())

			_, err := imgClient.Config(ctx, creds, imgRef)
			Expect(err).To(MatchError(ContainSubstring("MANIFEST_UNKNOWN")))
		})

		When("the secret doesn't exist", func() {
			BeforeEach(func() {
				creds.SecretName = "not-a-secret"
			})

			It("fails to authenticate", func() {
				Expect(testErr).To(MatchError(ContainSubstring("UNAUTHORIZED")))
			})
		})

		When("using a service account for secrets", func() {
			BeforeEach(func() {
				creds.SecretName = ""
				creds.ServiceAccountName = serviceAccountName
			})

			It("deletes an image", func() {
				Expect(testErr).NotTo(HaveOccurred())

				_, err := imgClient.Config(ctx, creds, imgRef)
				Expect(err).To(MatchError(ContainSubstring("MANIFEST_UNKNOWN")))
			})
		})

		When("simulating ECR", func() {
			BeforeEach(func() {
				pushRef = strings.Replace(noAuthRegistryServer.URL+"/foo/bar", "http://", "", 1)
				_, err := zipFile.Seek(0, 0)
				Expect(err).NotTo(HaveOccurred())

				creds.SecretName = ""
				imgRef, err = imgClient.Push(ctx, creds, pushRef, zipFile)
				Expect(err).NotTo(HaveOccurred())
			})

			When("the secret name is empty", func() {
				BeforeEach(func() {
					imgClient = image.NewClient(k8sClientset)
				})

				It("succeeds", func() {
					Expect(testErr).NotTo(HaveOccurred())

					_, err := imgClient.Config(ctx, creds, imgRef)
					Expect(err).To(MatchError(ContainSubstring("MANIFEST_UNKNOWN")))
				})
			})
		})
	})

	for _, reg := range registries {
		Describe("Delete from cloud registries", func() {
			reg := reg // capture reg so we don't just use the last one!
			var repoName string

			BeforeEach(func() {
				pushRef = reg.RepoName
				if pushRef == "" {
					repoName = testutils.GenerateGUID()
					pushRef = reg.PathPrefix + "/" + repoName
				}
				var err error

				imgRef, err = imgClient.Push(ctx, creds, pushRef, zipFile, "jim", "bob")
				Expect(err).NotTo(HaveOccurred())
			})

			JustBeforeEach(func() {
				testErr = imgClient.Delete(ctx, creds, imgRef)
			})

			It("deletes an image", func() {
				Expect(testErr).NotTo(HaveOccurred())

				_, err := imgClient.Config(ctx, creds, imgRef)
				Expect(err).To(MatchError(ContainSubstring("MANIFEST_UNKNOWN")))
			})
		})
	}
})

func pushImgWithLabelsAndPorts(repoRef string, labels map[string]string, ports []string) {
	portsMap := map[string]struct{}{}
	for _, port := range ports {
		portsMap[port] = struct{}{}
	}

	image, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{
		Config: v1.Config{
			Labels:       labels,
			ExposedPorts: portsMap,
		},
	})
	Expect(err).NotTo(HaveOccurred())

	ref, err := name.ParseReference(repoRef)
	Expect(err).NotTo(HaveOccurred())

	Expect(remote.Write(ref, image, remote.WithAuth(&authn.Basic{
		Username: "user",
		Password: "password",
	}))).To(Succeed())
}
