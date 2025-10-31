package main

import (
	"context"
	"fmt"
	"os"
	"runtime"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/controllers/coordination"
	bindingswebhook "code.cloudfoundry.org/korifi/controllers/webhooks/services/bindings"
	"code.cloudfoundry.org/korifi/migration/migration"
	"code.cloudfoundry.org/korifi/tools"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	err := korifiv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(fmt.Sprintf("could not add to scheme: %v", err))
	}

	k8sClientConfig := ctrl.GetConfigOrDie()
	k8sClient, err := client.New(k8sClientConfig, client.Options{})
	if err != nil {
		panic(fmt.Errorf("failed to create k8s client: %w", err))
	}

	korifiVersion, ok := os.LookupEnv("KORIFI_VERSION")
	if !ok {
		panic("KORIFI_VERSION must be set")
	}

	workersCount := tools.Max(1, runtime.NumCPU()/2)

	migrator := migration.New(k8sClient, korifiVersion, workersCount, coordination.NewNameRegistry(k8sClient, bindingswebhook.ServiceBindingEntityType))
	err = migrator.Run(context.Background())
	if err != nil {
		panic(err)
	}
}
