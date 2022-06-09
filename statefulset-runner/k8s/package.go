package k8s

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate sigs.k8s.io/controller-runtime/pkg/client.Client
//counterfeiter:generate sigs.k8s.io/controller-runtime/pkg/client.StatusWriter
