/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers/fake"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	clusterBuilderName = "my-amazing-cluster-builder"
)

var (
	ctx                     context.Context
	stopManager             context.CancelFunc
	stopClientCache         context.CancelFunc
	adminClient             client.Client
	testEnv                 *envtest.Environment
	fakeImageConfigGetter   *fake.ImageConfigGetter
	fakeImageDeleter        *fake.ImageDeleter
	buildWorkloadReconciler *k8s.PatchingReconciler[korifiv1alpha1.BuildWorkload, *korifiv1alpha1.BuildWorkload]
	rootNamespace           *v1.Namespace
	imageRepoCreator        *fake.RepositoryCreator
)

func TestAPIs(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(200 * time.Millisecond)
	SetDefaultConsistentlyDuration(10 * time.Second)
	SetDefaultConsistentlyPollingInterval(200 * time.Millisecond)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))
	ctx = context.Background()

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "helm", "korifi", "controllers", "crds"),
			filepath.Join("..", "..", "tests", "vendor", "kpack"),
		},
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{
				filepath.Join("..", "..", "helm", "korifi", "kpack-image-builder", "manifests.yaml"),
			},
		},
		ErrorIfCRDPathMissing: true,
	}

	_, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(buildv1alpha2.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sManager := helpers.NewK8sManager(testEnv, filepath.Join("helm", "korifi", "kpack-image-builder", "role.yaml"))
	adminClient, stopClientCache = helpers.NewCachedClient(testEnv.Config)

	finalizer.NewKpackImageBuilderFinalizerWebhook().SetupWebhookWithManager(k8sManager)

	controllerConfig := &config.ControllerConfig{
		CFRootNamespace:           PrefixedGUID("cf"),
		ClusterBuilderName:        "cf-kpack-builder",
		ContainerRepositoryPrefix: "image/registry/tag",
		BuilderServiceAccount:     "builder-service-account",
		CFStagingResources: config.CFStagingResources{
			BuildCacheMB: 1024,
			DiskMB:       2048,
			MemoryMB:     1234,
		},
	}

	imageRepoCreator = new(fake.RepositoryCreator)
	fakeImageConfigGetter = new(fake.ImageConfigGetter)
	buildWorkloadReconciler = controllers.NewBuildWorkloadReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		ctrl.Log.WithName("kpack-image-builder").WithName("BuildWorkload"),
		controllerConfig,
		fakeImageConfigGetter,
		"my.repository/my-prefix/",
		imageRepoCreator,
		4*time.Second,
	)
	err = buildWorkloadReconciler.SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	Expect(
		controllers.NewBuilderInfoReconciler(
			k8sManager.GetClient(),
			k8sManager.GetScheme(),
			ctrl.Log.WithName("kpack-image-builder").WithName("BuilderInfo"),
			clusterBuilderName,
			controllerConfig.CFRootNamespace,
		).SetupWithManager(k8sManager),
	).To(Succeed())

	fakeImageDeleter = new(fake.ImageDeleter)
	kpackBuildReconciler := controllers.NewKpackBuildController(
		k8sManager.GetClient(),
		ctrl.Log.WithName("kpack-image-builder").WithName("KpackBuild"),
		fakeImageDeleter,
		"builder-service-account",
	)
	err = kpackBuildReconciler.SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	stopManager = helpers.StartK8sManager(k8sManager)

	rootNamespace = &v1.Namespace{
		ObjectMeta: ctrl.ObjectMeta{
			Name: controllerConfig.CFRootNamespace,
		},
	}
	Expect(adminClient.Create(ctx, rootNamespace)).To(Succeed())

	// create a test storage class that can't be resized
	Expect(adminClient.Create(ctx, &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "non-resizable-class",
		},
		Provisioner:          "some-fancy-provisioner",
		AllowVolumeExpansion: tools.PtrTo(false),
	})).To(Succeed())
})

var _ = AfterSuite(func() {
	stopClientCache()
	stopManager()
	Expect(testEnv.Stop()).To(Succeed())
})
