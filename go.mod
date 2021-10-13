module code.cloudfoundry.org/cf-k8s-controllers

go 1.16

require (
	github.com/buildpacks/lifecycle v0.12.0
	github.com/go-logr/logr v0.4.0
	github.com/google/go-containerregistry v0.5.2-0.20210604130445-3bfab55f3bd9
	github.com/google/uuid v1.2.0
	github.com/hashicorp/go-uuid v1.0.1
	github.com/maxbrunsfeld/counterfeiter/v6 v6.4.1
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/pivotal/kpack v0.3.1
	github.com/projectcontour/contour v1.18.2
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/client-go v0.22.2
	sigs.k8s.io/controller-runtime v0.9.2
)
