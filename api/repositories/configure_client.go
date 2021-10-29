package repositories

import (
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func BuildK8sClient(config *rest.Config) (k8sclient.Interface, error) {
	return k8sclient.NewForConfig(config)
}

func BuildCRClient(config *rest.Config) (crclient.Client, error) {
	return crclient.New(config, crclient.Options{Scheme: scheme.Scheme})
}
