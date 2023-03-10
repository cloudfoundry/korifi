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
	)

	BeforeEach(func() {
		testErr = nil
		var err error
		zipFile, err = os.Open("fixtures/layer.zip")
		Expect(err).NotTo(HaveOccurred())

		pushRef = strings.Replace(authRegistryServer.URL+"/foo/bar", "http://", "", 1)

		imgClient = image.NewClient(k8sClientset, "default", secretName)
	})

	Describe("Push", func() {
		JustBeforeEach(func() {
			imgRef, testErr = imgClient.Push(ctx, pushRef, zipFile)
		})

		It("pushes a zip archive as an image to the registry", func() {
			Expect(testErr).NotTo(HaveOccurred())
			Expect(imgRef).To(HavePrefix(pushRef))

			labels, err := imgClient.Labels(ctx, imgRef)
			Expect(err).NotTo(HaveOccurred())
			Expect(labels).To(BeEmpty())
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
				imgClient = image.NewClient(k8sClientset, "default", "another-secret-name")
			})

			It("fails to authenticate", func() {
				Expect(testErr).To(MatchError(ContainSubstring("Unauthorized")))
			})
		})

		When("simulating ECR", func() {
			BeforeEach(func() {
				pushRef = strings.Replace(noAuthRegistryServer.URL+"/foo/bar", "http://", "", 1)
			})

			When("the secret name is empty", func() {
				BeforeEach(func() {
					imgClient = image.NewClient(k8sClientset, "default", "")
				})

				It("succeeds", func() {
					Expect(testErr).NotTo(HaveOccurred())
					Expect(imgRef).To(HavePrefix(pushRef))
				})
			})
		})
	})

	Describe("Labels", func() {
		var labels map[string]string

		BeforeEach(func() {
			pushRef += "/with/labels"
			pushImgWithLabels(pushRef, map[string]string{"foo": "bar"})
		})

		JustBeforeEach(func() {
			labels, testErr = imgClient.Labels(ctx, pushRef)
		})

		It("fetches the image labels", func() {
			Expect(labels).To(Equal(map[string]string{"foo": "bar"}))
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
				imgClient = image.NewClient(k8sClientset, "default", "another-secret-name")
			})

			It("fails to authenticate", func() {
				Expect(testErr).To(MatchError(ContainSubstring("UNAUTHORIZED")))
			})
		})

		When("simulating ECR", func() {
			BeforeEach(func() {
				pushRef = strings.Replace(noAuthRegistryServer.URL+"/foo/bar/with/labels", "http://", "", 1)
				pushImgWithLabels(pushRef, map[string]string{"foo": "bar"})
			})

			When("the secret name is empty", func() {
				BeforeEach(func() {
					imgClient = image.NewClient(k8sClientset, "default", "")
				})

				It("succeeds", func() {
					Expect(testErr).NotTo(HaveOccurred())
					Expect(labels).To(Equal(map[string]string{"foo": "bar"}))
				})
			})
		})
	})

	Describe("Delete", func() {
		BeforeEach(func() {
			var err error
			imgRef, err = imgClient.Push(ctx, pushRef, zipFile, "jim", "bob")
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			testErr = imgClient.Delete(ctx, imgRef)
		})

		It("deletes an image", func() {
			Expect(testErr).NotTo(HaveOccurred())

			_, err := imgClient.Labels(ctx, imgRef)
			Expect(err).To(MatchError(ContainSubstring("MANIFEST_UNKNOWN")))
		})

		When("the secret doesn't exist", func() {
			BeforeEach(func() {
				imgClient = image.NewClient(k8sClientset, "default", "another-secret-name")
			})

			It("fails to authenticate", func() {
				Expect(testErr).To(MatchError(ContainSubstring("UNAUTHORIZED")))
			})
		})

		When("simulating ECR", func() {
			BeforeEach(func() {
				pushRef = strings.Replace(noAuthRegistryServer.URL+"/foo/bar", "http://", "", 1)
				_, err := zipFile.Seek(0, 0)
				Expect(err).NotTo(HaveOccurred())

				imgRef, err = imgClient.Push(ctx, pushRef, zipFile)
				Expect(err).NotTo(HaveOccurred())
			})

			When("the secret name is empty", func() {
				BeforeEach(func() {
					imgClient = image.NewClient(k8sClientset, "default", "")
				})

				It("succeeds", func() {
					Expect(testErr).NotTo(HaveOccurred())

					_, err := imgClient.Labels(ctx, imgRef)
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

				imgRef, err = imgClient.Push(ctx, pushRef, zipFile, "jim", "bob")
				Expect(err).NotTo(HaveOccurred())
			})

			JustBeforeEach(func() {
				testErr = imgClient.Delete(ctx, imgRef)
			})

			It("deletes an image", func() {
				Expect(testErr).NotTo(HaveOccurred())

				_, err := imgClient.Labels(ctx, imgRef)
				Expect(err).To(MatchError(ContainSubstring("MANIFEST_UNKNOWN")))
			})
		})
	}
})

func pushImgWithLabels(repoRef string, labels map[string]string) {
	image, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{
		Config: v1.Config{
			Labels: labels,
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
