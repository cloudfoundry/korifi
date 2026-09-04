package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
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

	labelSigningSecretPath := os.Getenv("LABEL_SIGNING_SECRET_PATH")
	if labelSigningSecretPath == "" {
		labelSigningSecretPath = "/etc/korifi-label-signing-secret/key"
	}
	labelSigningSecret, err := os.ReadFile(filepath.Clean(labelSigningSecretPath))
	if err != nil {
		panic(fmt.Sprintf("could not read label signing secret: %v", err))
	}

	workersCount := tools.Max(1, runtime.NumCPU()/2)

	migrator := migration.New(k8sClient, korifiVersion, labelSigningSecret, workersCount)
	err = migrator.Run(context.Background())
	if err != nil {
		panic(err)
	}
}
