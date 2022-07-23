# Image URL to use all building/pushing image targets
IMG_CONTROLLERS ?= cloudfoundry/korifi-controllers:latest
IMG_API ?= cloudfoundry/korifi-api:latest
CRD_OPTIONS ?= "crd"

# Run controllers tests with two nodes by default to (potentially) minimise
# flakes.
CONTROLLERS_GINKGO_NODES ?= 2
ifdef GINKGO_NODES
CONTROLLERS_GINKGO_NODES = $(GINKGO_NODES)
endif

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

manifests-controllers: install-controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./controllers/..." output:crd:artifacts:config=controllers/config/crd/bases output:rbac:artifacts:config=controllers/config/rbac output:webhook:artifacts:config=controllers/config/webhook

manifests-api: install-controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=system-clusterrole paths=./api/... output:rbac:artifacts:config=api/config/base/rbac

generate-controllers: install-controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="controllers/hack/boilerplate.go.txt" paths="./controllers/..."

generate-fakes:
	go generate ./...

fmt: install-gofumpt install-shfmt
	$(GOFUMPT) -w .
	$(SHFMT) -w -i 2 -ci .

vet: ## Run go vet against code.
	go vet ./...

lint:
	golangci-lint run -v

test: lint test-controllers-api test-kpack-image-builder test-statefulset-runner test-e2e

test-controllers-api: test-controllers test-api

test-controllers: install-ginkgo manifests-controllers generate-controllers fmt vet ## Run tests.
	cd controllers && GINKGO_NODES=$(CONTROLLERS_GINKGO_NODES) ../scripts/run-tests.sh

test-api: install-ginkgo manifests-api fmt vet
	cd api && ../scripts/run-tests.sh --skip-package=test

test-e2e: install-ginkgo
	./scripts/run-tests.sh tests/e2e

test-kpack-image-builder:
	make -C kpack-image-builder test

test-statefulset-runner:
	make -C statefulset-runner test

##@ Build

build: generate-controllers fmt vet ## Build manager binary.
	go build -o controllers/bin/manager controllers/main.go

run-controllers: manifests-controllers generate-controllers fmt vet ## Run a controller from your host.
	CONTROLLERSCONFIG=$(shell pwd)/controllers/config/base/controllersconfig ENABLE_WEBHOOKS=false go run ./controllers/main.go

run-api: fmt vet
	APICONFIG=$(shell pwd)/api/config/base/apiconfig go run ./api/main.go

docker-build: docker-build-controllers docker-build-kpack-image-builder docker-build-statefulset-runner docker-build-api

docker-build-api:
	docker buildx build --load -f api/Dockerfile -t ${IMG_API} .

docker-build-api-debug:
	docker buildx build --load -f api/remote-debug/Dockerfile -t ${IMG_API} .

docker-build-controllers:
	docker buildx build --load -f controllers/Dockerfile -t ${IMG_CONTROLLERS} .

docker-build-controllers-debug:
	docker buildx build --load -f controllers/remote-debug/Dockerfile -t ${IMG_CONTROLLERS} .

docker-build-kpack-image-builder:
	make -C kpack-image-builder docker-build

docker-build-kpack-image-builder-debug:
	make -C kpack-image-builder docker-build-debug

docker-build-statefulset-runner:
	make -C statefulset-runner docker-build

docker-build-statefulset-runner-debug:
	make -C statefulset-runner docker-build-debug

docker-push: docker-push-controllers docker-push-kpack-image-builder docker-push-statefulset-runner docker-push-api

docker-push-api:
	docker push ${IMG_API}

docker-push-controllers:
	docker push ${IMG_CONTROLLERS}

docker-push-kpack-image-builder:
	make -C kpack-image-builder docker-push

docker-push-statefulset-runner:
	make -C statefulset-runner docker-push

kind-load-images: kind-load-controllers-image kind-load-kpack-image-builder-image kind-load-statefulset-runner-image kind-load-api-image

kind-load-api-image:
	kind load docker-image ${IMG_API}

kind-load-controllers-image:
	kind load docker-image ${IMG_CONTROLLERS}

kind-load-kpack-image-builder-image:
	make -C kpack-image-builder kind-load-image

kind-load-statefulset-runner-image:
	make -C statefulset-runner kind-load-image

##@ Deployment

install-crds: manifests-controllers install-kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build controllers/config/crd | kapp deploy -a korifi-crds -f - -y

uninstall-crds: manifests-controllers install-kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	kapp delete -a korifi-crds -y

deploy: deploy-controllers deploy-kpack-image-builder deploy-statefulset-runner deploy-api

deploy-kind: deploy-controllers deploy-api-kind

deploy-kind-local: deploy-controllers-kind-local deploy-kpack-image-builder-kind-local deploy-statefulset-runner deploy-api-kind-local

deploy-api: set-image-ref-api
	$(KUSTOMIZE) build api/config/base | kapp deploy -a korifi-api -f - -y

deploy-api-kind: set-image-ref-api
	$(KUSTOMIZE) build api/config/overlays/kind | kapp deploy -a korifi-api -f - -y

deploy-api-kind-local: set-image-ref-api
	$(KUSTOMIZE) build api/config/overlays/kind-local-registry | kapp deploy -a korifi-api -f - -y

deploy-api-kind-local-debug: set-image-ref-api
	$(KUSTOMIZE) build api/config/overlays/kind-api-debug | kapp deploy -a korifi-api -f - -y

deploy-controllers: set-image-ref-controllers
	$(KUSTOMIZE) build controllers/config/default | kapp deploy -a korifi-controllers -f - -y

deploy-controllers-kind-local: set-image-ref-controllers
	$(KUSTOMIZE) build controllers/config/overlays/kind-local-registry | kapp deploy -a korifi-controllers -f - -y

deploy-controllers-kind-local-debug: set-image-ref-controllers
	$(KUSTOMIZE) build controllers/config/overlays/kind-controller-debug | kapp deploy -a korifi-controllers -f - -y

deploy-kpack-image-builder:
	make -C kpack-image-builder deploy

deploy-kpack-image-builder-kind-local:
	make -C kpack-image-builder deploy-kind-local

deploy-kpack-image-builder-kind-local-debug:
	make -C kpack-image-builder deploy-kind-local-debug

deploy-statefulset-runner:
	make -C statefulset-runner deploy

deploy-statefulset-runner-kind-local-debug:
	make -C statefulset-runner deploy-kind-local-debug

undeploy: undeploy-api undeploy-controllers undeploy-kpack-image-builder undeploy-statefulset-runner

undeploy-api: ## Undeploy api from the K8s cluster specified in ~/.kube/config.
	kapp delete -a korifi-api -y

undeploy-controllers: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	kapp delete -a korifi-controllers -y

undeploy-kpack-image-builder:
	make -C kpack-image-builder undeploy

undeploy-statefulset-runner:
	make -C statefulset-runner undeploy

set-image-ref: set-image-ref-api set-image-ref-controllers set-image-ref-kpack-image-builder set-image-ref-statefulset-runner

set-image-ref-api: manifests-api install-kustomize
	cd api/config/base && $(KUSTOMIZE) edit set image cloudfoundry/korifi-api=${IMG_API}

set-image-ref-controllers: manifests-controllers install-kustomize
	cd controllers/config/manager && $(KUSTOMIZE) edit set image cloudfoundry/korifi-controllers=${IMG_CONTROLLERS}

set-image-ref-kpack-image-builder:
	make -C kpack-image-builder set-image-ref

set-image-ref-statefulset-runner:
	make -C statefulset-runner set-image-ref

CONTROLLER_GEN = $(shell pwd)/controllers/bin/controller-gen
install-controller-gen: ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.9.2)

KUSTOMIZE = $(shell pwd)/controllers/bin/kustomize
install-kustomize: ## Download kustomize locally if necessary.
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.2)

GOFUMPT = $(shell go env GOPATH)/bin/gofumpt
install-gofumpt:
	go install mvdan.cc/gofumpt@latest

SHFMT = $(shell go env GOPATH)/bin/shfmt
install-shfmt:
	go install mvdan.cc/sh/v3/cmd/shfmt@latest

install-ginkgo:
	go install github.com/onsi/ginkgo/v2/ginkgo

# go-install-tool will 'go get' any package $2 and install it to $1.
define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$$(dirname $(CONTROLLER_GEN)) go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef
