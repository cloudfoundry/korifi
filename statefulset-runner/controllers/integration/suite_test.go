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

package integration_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	. "code.cloudfoundry.org/korifi/statefulset-runner/controllers"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"go.uber.org/zap/zapcore"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	stopManager     context.CancelFunc
	stopClientCache context.CancelFunc
	k8sClient       client.Client
	testEnv         *envtest.Environment
)

func TestAppWorkloadsController(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(10 * time.Second)
	SetDefaultEventuallyPollingInterval(200 * time.Millisecond)

	RunSpecs(t, "Controller Integration Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))

	Expect(korifiv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "..", "helm", "korifi", "controllers", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	_, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())

	k8sManager := helpers.NewK8sManager(testEnv, filepath.Join("helm", "korifi", "statefulset-runner", "role.yaml"))
	k8sClient, stopClientCache = helpers.NewCachedClient(testEnv.Config)

	appWorkloadReconciler := NewAppWorkloadReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		NewAppWorkloadToStatefulsetConverter(k8sManager.GetScheme()),
		NewPDBUpdater(k8sManager.GetClient()),
		ctrl.Log.WithName("statefulset-runner").WithName("AppWorkload"),
	)
	err = appWorkloadReconciler.SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	runnerInfoReconciler := NewRunnerInfoReconciler(
		k8sManager.GetClient(),
		k8sManager.GetScheme(),
		ctrl.Log.WithName("statefulset-runner").WithName("RunnerInfo"),
	)
	err = runnerInfoReconciler.SetupWithManager(k8sManager)
	Expect(err).NotTo(HaveOccurred())

	stopManager = helpers.StartK8sManager(k8sManager)
})

var _ = AfterSuite(func() {
	stopClientCache()
	stopManager()
	Expect(testEnv.Stop()).To(Succeed())
})
