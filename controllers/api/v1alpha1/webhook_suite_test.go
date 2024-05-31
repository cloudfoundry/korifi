/*

Copyright 2021.

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

package v1alpha1_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/controllers/shared"
	"code.cloudfoundry.org/korifi/controllers/coordination"
	"code.cloudfoundry.org/korifi/controllers/webhooks/finalizer"
	"code.cloudfoundry.org/korifi/controllers/webhooks/networking/domains"
	"code.cloudfoundry.org/korifi/controllers/webhooks/networking/routes"
	"code.cloudfoundry.org/korifi/controllers/webhooks/validation"
	"code.cloudfoundry.org/korifi/controllers/webhooks/version"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/apps"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/orgs"
	packageswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/workloads/packages"
	"code.cloudfoundry.org/korifi/controllers/webhooks/workloads/spaces"
	"code.cloudfoundry.org/korifi/tests/helpers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	defaultMemoryMB    = 128
	defaultDiskQuotaMB = 256
	defaultTimeout     = 60
)

var (
	ctx             context.Context
	stopManager     context.CancelFunc
	stopClientCache context.CancelFunc
	testEnv         *envtest.Environment
	adminClient     client.Client
	namespace       string
)

func TestKorifiMutatingWebhooks(t *testing.T) {
	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(250 * time.Millisecond)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Korifi Mutating Webhooks Integration Test Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, stopManager = context.WithCancel(context.TODO())

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "helm", "korifi", "controllers", "crds")},
		ErrorIfCRDPathMissing: false,
		WebhookInstallOptions: envtest.WebhookInstallOptions{
			Paths: []string{filepath.Join("..", "..", "..", "helm", "korifi", "controllers", "manifests.yaml")},
		},
	}

	namespace = uuid.NewString()

	_, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	Expect(corev1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(admissionv1beta1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(coordinationv1.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sManager := helpers.NewK8sManager(testEnv, filepath.Join("helm", "korifi", "controllers", "role.yaml"))
	Expect(shared.SetupIndexWithManager(k8sManager)).To(Succeed())

	adminClient, stopClientCache = helpers.NewCachedClient(testEnv.Config)

	uncachedClient := helpers.NewUncachedClient(k8sManager.GetConfig())
	Expect((&korifiv1alpha1.CFApp{}).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect(apps.NewValidator(
		validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, apps.AppEntityType)),
	).SetupWebhookWithManager(k8sManager)).To(Succeed())

	Expect((&korifiv1alpha1.CFRoute{}).SetupWebhookWithManager(k8sManager)).To(Succeed())
	Expect(routes.NewValidator(
		validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, routes.RouteEntityType)),
		namespace,
		uncachedClient,
	).SetupWebhookWithManager(k8sManager)).To(Succeed())

	Expect(domains.NewValidator(uncachedClient).SetupWebhookWithManager(k8sManager)).To(Succeed())

	Expect((&korifiv1alpha1.CFPackage{}).SetupWebhookWithManager(k8sManager)).To(Succeed())

	Expect(korifiv1alpha1.NewCFProcessDefaulter(defaultMemoryMB, defaultDiskQuotaMB, defaultTimeout).
		SetupWebhookWithManager(k8sManager)).To(Succeed())

	Expect((&korifiv1alpha1.CFBuild{}).SetupWebhookWithManager(k8sManager)).To(Succeed())

	orgNameDuplicateValidator := validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, orgs.CFOrgEntityType))
	orgPlacementValidator := validation.NewPlacementValidator(uncachedClient, namespace)
	Expect(orgs.NewValidator(orgNameDuplicateValidator, orgPlacementValidator).SetupWebhookWithManager(k8sManager)).To(Succeed())

	spaceNameDuplicateValidator := validation.NewDuplicateValidator(coordination.NewNameRegistry(uncachedClient, spaces.CFSpaceEntityType))
	spacePlacementValidator := validation.NewPlacementValidator(uncachedClient, namespace)
	Expect(spaces.NewValidator(spaceNameDuplicateValidator, spacePlacementValidator).SetupWebhookWithManager(k8sManager)).To(Succeed())
	version.NewVersionWebhook("some-version").SetupWebhookWithManager(k8sManager)
	finalizer.NewControllersFinalizerWebhook().SetupWebhookWithManager(k8sManager)
	Expect(packageswebhook.NewValidator().SetupWebhookWithManager(k8sManager)).To(Succeed())

	Expect(adminClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	})).To(Succeed())

	stopManager = helpers.StartK8sManager(k8sManager)
})

var _ = AfterSuite(func() {
	stopManager()
	stopClientCache()
	Expect(testEnv.Stop()).To(Succeed())
})
