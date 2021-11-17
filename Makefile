# Image URL to use all building/pushing image targets
IMG_CONTROLLERS ?= cloudfoundry/cf-k8s-controllers:latest
IMG_API ?= cloudfoundry/cf-k8s-api:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

manifests: manifests-controllers manifests-api

manifests-controllers: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./controllers/..." output:crd:artifacts:config=controllers/config/crd/bases output:rbac:artifacts:config=controllers/config/rbac output:webhook:artifacts:config=controllers/config/webhook

manifests-api: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=cf-admin-clusterrole paths=./api/... output:rbac:artifacts:config=api/config/base/rbac

generate-controllers: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="controllers/hack/boilerplate.go.txt" paths="./controllers/..."

generate-fakes:
	go generate ./...

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

test: test-controllers-api test-e2e

test-controllers-api: test-controllers test-api

test-unit: test-controllers test-api-unit

test-controllers: ginkgo manifests-controllers generate-controllers fmt vet ## Run tests.
	cd controllers && ../scripts/run-tests.sh

test-api: test-api-unit test-api-integration

test-api-unit: ginkgo fmt vet
	cd api && ../scripts/run-tests.sh --skip-package=test

test-api-integration: ginkgo
	cd api && ../scripts/run-tests.sh tests/integration

test-e2e: ginkgo
	cd api && ../scripts/run-tests.sh tests/e2e

##@ Build

build: generate-controllers fmt vet ## Build manager binary.
	go build -o controllers/bin/manager controllers/main.go

run-controllers: manifests-controllers generate-controllers fmt vet ## Run a controller from your host.
	CONTROLLERSCONFIG=$(shell pwd)/controllers/config/base/controllersconfig ENABLE_WEBHOOKS=false go run ./controllers/main.go

run-api: fmt vet
	APICONFIG=$(shell pwd)/api/config/base/apiconfig go run ./api/main.go

docker-build: docker-build-controllers docker-build-api

docker-build-controllers:
	docker buildx build --load -f controllers/Dockerfile -t ${IMG_CONTROLLERS} .

docker-build-api:
	docker buildx build --load -f api/Dockerfile -t ${IMG_API} .

docker-push: docker-push-controllers docker-push-api

docker-push-controllers:
	docker push ${IMG_CONTROLLERS}

docker-push-api:
	docker push ${IMG_API}

kind-load-images: kind-load-controllers-image kind-load-api-image

kind-load-controllers-image:
	kind load docker-image ${IMG_CONTROLLERS}

kind-load-api-image:
	kind load docker-image ${IMG_API}

##@ Deployment

install-crds: manifests-controllers kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build controllers/config/crd | kubectl apply -f -

uninstall-crds: manifests-controllers kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build controllers/config/crd | kubectl delete -f -

deploy: install-crds deploy-controllers deploy-api

deploy-kind: install-crds deploy-controllers deploy-api-kind-auth

deploy-controllers: manifests-controllers kustomize
	cd controllers/config/manager && $(KUSTOMIZE) edit set image cloudfoundry/cf-k8s-controllers=${IMG_CONTROLLERS}
	$(KUSTOMIZE) build controllers/config/default | kubectl apply -f -

deploy-api: kustomize
	cd api/config/base && $(KUSTOMIZE) edit set image cloudfoundry/cf-k8s-api=${IMG_API}
	$(KUSTOMIZE) build api/config/base | kubectl apply -f -

deploy-api-kind-auth: kustomize
	cd api/config/base && $(KUSTOMIZE) edit set image cloudfoundry/cf-k8s-api=${IMG_API}
	$(KUSTOMIZE) build api/config/overlays/kind-auth-enabled | kubectl apply -f -

undeploy-controllers: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build controllers/config/default | kubectl delete -f -

undeploy-api: ## Undeploy api from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build api/config/base | kubectl delete -f -

build-reference: build-reference-controllers build-reference-api

build-reference-controllers: manifests-controllers kustomize ## Generate reference yaml and output to ./reference/cf-k8s-controllers.yaml
	cd controllers/config/manager && $(KUSTOMIZE) edit set image cloudfoundry/cf-k8s-controllers=${IMG_CONTROLLERS}
	$(KUSTOMIZE) build controllers/config/default -o controllers/reference/cf-k8s-controllers.yaml

build-reference-api: manifests-api kustomize
	cd api/config/base && $(KUSTOMIZE) edit set image cloudfoundry/cf-k8s-api=${IMG_API}
	$(KUSTOMIZE) build api/config/base -o api/reference/cf-k8s-api.yaml

CONTROLLER_GEN = $(shell pwd)/controllers/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.5.0)

KUSTOMIZE = $(shell pwd)/controllers/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.2.0)

ginkgo:
	go install github.com/onsi/ginkgo/ginkgo

# go-get-tool will 'go get' any package $2 and install it to $1.
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$$(dirname $(CONTROLLER_GEN)) go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef
