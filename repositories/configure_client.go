package repositories

import (
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func BuildClient(config *rest.Config) (client.Client, error) {
	return client.New(config, client.Options{Scheme: scheme.Scheme})
}
