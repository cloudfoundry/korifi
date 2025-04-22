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
	"os"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers/config"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers/fake"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	controllersconfig "code.cloudfoundry.org/korifi/controllers/config"
	"github.com/google/uuid"
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
	"sigs.k8s.io/controller-runtime/pkg/manager"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

var (
	ctx                     context.Context
	stopManager             context.CancelFunc
	stopClientCache         context.CancelFunc
	adminClient             client.Client
	testEnv                 *envtest.Environment
	fakeImageConfigGetter   *fake.ImageConfigGetter
	fakeImageDeleter        *fake.ImageDeleter
	buildWorkloadReconciler *k8s.PatchingReconciler[korifiv1alpha1.BuildWorkload]
	rootNamespace           *v1.Namespace
	imageRepoCreator        *fake.RepositoryCreator
	k8sManager              manager.Manager
	clusterBuilderName      string
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

	webhookManifestsPath := helpers.GenerateWebhookManifest(
		"code.cloudfoundry.org/korifi/kpack-image-builder/controllers/webhooks/finalizer",
	)
	DeferCleanup(func() {
		Expect(os.RemoveAll(filepath.Dir(webhookManifestsPath))).To(Succeed())
	})
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "helm", "korifi", "controllers", "crds"),
			filepath.Join("..", "..", "tests", "vendor", "kpack"),
		},
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{webhookManifestsPath},
		},
		ErrorIfCRDPathMissing: true,
	}

	_, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(buildv1alpha2.AddToScheme(scheme.Scheme)).To(Succeed())

	testEnvClient, err := client.New(testEnv.Config, client.Options{
		Scheme: scheme.Scheme,
	})
	Expect(err).NotTo(HaveOccurred())

	// create a test storage class that can't be resized
	Expect(testEnvClient.Create(ctx, &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "non-resizable-class",
		},
		Provisioner:          "some-fancy-provisioner",
		AllowVolumeExpansion: tools.PtrTo(false),
	})).To(Succeed())
})

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).To(Succeed())
})

var _ = BeforeEach(func() {
	adminClient, stopClientCache = helpers.NewCachedClient(testEnv.Config)
	k8sManager = helpers.NewK8sManager(testEnv, filepath.Join("helm", "korifi", "kpack-image-builder", "role.yaml"))

	finalizer.NewKpackImageBuilderFinalizerWebhook().SetupWebhookWithManager(k8sManager)

	rootNamespace = &v1.Namespace{
		ObjectMeta: ctrl.ObjectMeta{
			Name: uuid.NewString(),
		},
	}
	Expect(adminClient.Create(ctx, rootNamespace)).To(Succeed())

	clusterBuilderName = uuid.NewString()
	controllerConfig := &config.Config{
		CFRootNamespace:           rootNamespace.Name,
		ClusterBuilderName:        clusterBuilderName,
		BuilderServiceAccount:     "builder-service-account",
		BuilderReadinessTimeout:   4 * time.Second,
		ContainerRepositoryPrefix: "my.repository/my-prefix/",
		CFStagingResources: controllersconfig.CFStagingResources{
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
		imageRepoCreator,
	)

	Expect(buildWorkloadReconciler.SetupWithManager(k8sManager)).To(Succeed())
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
	Expect(kpackBuildReconciler.SetupWithManager(k8sManager)).To(Succeed())

	stopManager = helpers.StartK8sManager(k8sManager)
})

var _ = AfterEach(func() {
	stopClientCache()
	stopManager()
})
