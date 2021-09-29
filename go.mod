module code.cloudfoundry.org/cf-k8s-api

go 1.16

require (
	code.cloudfoundry.org/cf-k8s-controllers v0.0.0-20210927185152-baa2416c14c0
	github.com/go-logr/logr v0.4.0
	github.com/go-playground/locales v0.14.0
	github.com/go-playground/universal-translator v0.18.0
	github.com/go-playground/validator/v10 v10.9.0
	github.com/google/uuid v1.1.2
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/go-uuid v1.0.1
	github.com/maxbrunsfeld/counterfeiter/v6 v6.4.1
	github.com/onsi/gomega v1.15.0
	github.com/sclevine/spec v1.4.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/component-base v0.22.2 // indirect
	sigs.k8s.io/controller-runtime v0.9.6
	sigs.k8s.io/controller-tools v0.6.2
	sigs.k8s.io/hierarchical-namespaces v0.0.0-20210827200453-b03328e734e6
)
