module code.cloudfoundry.org/cf-k8s-api

go 1.16

require (
	code.cloudfoundry.org/cf-k8s-controllers v0.0.0-20210902233618-f822f3073471
	github.com/go-logr/logr v0.4.0
	github.com/gorilla/mux v1.8.0
	github.com/onsi/gomega v1.15.0
	github.com/sclevine/spec v1.4.0
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	sigs.k8s.io/controller-runtime v0.9.6
	sigs.k8s.io/controller-tools v0.6.2
)
