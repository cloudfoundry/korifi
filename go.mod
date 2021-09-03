module code.cloudfoundry.org/cf-k8s-api

go 1.16

require (
	code.cloudfoundry.org/cf-k8s-controllers v0.0.0-20210826202621-aa5e1d3837a2
	github.com/Azure/go-autorest/autorest/adal v0.9.13 // indirect
	github.com/go-logr/logr v0.4.0
	github.com/gorilla/mux v1.8.0
	github.com/onsi/gomega v1.15.0
	github.com/sclevine/spec v1.4.0
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	sigs.k8s.io/controller-runtime v0.9.6
)
