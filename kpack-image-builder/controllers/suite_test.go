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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1alpha2 "github.com/pivotal/kpack/pkg/apis/build/v1alpha2"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/config"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers/fake"
	"code.cloudfoundry.org/korifi/tools/k8s"
	//+kubebuilder:scaffold:imports
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

const (
	clusterBuilderName = "my-amazing-cluster-builder"
)

var (
	cancel                  context.CancelFunc
	cfg                     *rest.Config
	k8sClient               client.Client
	testEnv                 *envtest.Environment
	fakeImageConfigGetter   *fake.ImageConfigGetter
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

	ctx, cancelFunc := context.WithCancel(context.TODO())
	cancel = cancelFunc

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "helm", "korifi", "controllers", "crds"),
			filepath.Join("..", "..", "tests", "vendor", "kpack"),
		},
		ErrorIfCRDPathMissing: true,
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = korifiv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = buildv1alpha2.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	webhookInstallOptions := &testEnv.WebhookInstallOptions
	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme.Scheme,
		Host:               webhookInstallOptions.LocalServingHost,
		Port:               webhookInstallOptions.LocalServingPort,
		CertDir:            webhookInstallOptions.LocalServingCertDir,
		LeaderElection:     false,
		MetricsBindAddress: "0",
	})
	Expect(err).NotTo(HaveOccurred())

	controllerConfig := &config.ControllerConfig{
		CFRootNamespace:           PrefixedGUID("cf"),
		ClusterBuilderName:        "cf-kpack-builder",
		ContainerRepositoryPrefix: "image/registry/tag",
		BuilderServiceAccount:     "builder-service-account",
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
	)
	err = (buildWorkloadReconciler).SetupWithManager(k8sManager)
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

	//+kubebuilder:scaffold:scheme

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	rootNamespace = &v1.Namespace{
		ObjectMeta: ctrl.ObjectMeta{
			Name: controllerConfig.CFRootNamespace,
		},
	}
	Expect(k8sClient.Create(ctx, rootNamespace)).To(Succeed())

	go func() {
		defer GinkgoRecover()
		err = k8sManager.Start(ctx)
		Expect(err).NotTo(HaveOccurred())
	}()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	Expect(testEnv.Stop()).To(Succeed())
})
