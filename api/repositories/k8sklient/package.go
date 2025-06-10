package k8sklient

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

//counterfeiter:generate -o fake -fake-name WithWatch sigs.k8s.io/controller-runtime/pkg/client.WithWatch
//counterfeiter:generate -o fake -fake-name WatchInterface k8s.io/apimachinery/pkg/watch.Interface
