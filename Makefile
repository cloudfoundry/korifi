# Image URL to use all building/pushing image targets
IMG_CONTROLLERS ?= cloudfoundry/korifi-controllers:latest
IMG_API ?= cloudfoundry/korifi-api:latest

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

manifests: manifests-api manifests-controllers manifests-job-task-runner manifests-kpack-image-builder manifests-statefulset-runner

manifests-api: install-controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) \
		paths=./api/... output:rbac:artifacts:config=helm/api/templates \
		rbac:roleName=korifi-api-system-role

	sed -i.bak -e 's/ROOT_NAMESPACE/{{ .Values.global.rootNamespace }}/' helm/api/templates/role.yaml
	rm -f helm/api/templates/role.yaml.bak

manifests-controllers: install-controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) \
		paths="./controllers/..." \
		crd \
		rbac:roleName=korifi-controllers-manager-role \
		webhook \
		output:crd:artifacts:config=helm/controllers/templates/crds \
		output:rbac:artifacts:config=helm/controllers/templates \
		output:webhook:artifacts:config=helm/controllers/templates

	sed -i.bak -e '/^metadata:.*/a \ \ annotations:\n    cert-manager.io/inject-ca-from: "{{ .Values.namespace }}/korifi-controllers-serving-cert"' helm/controllers/templates/manifests.yaml
	sed -i.bak -e 's/name: \(webhook-service\)/name: korifi-controllers-\1/' helm/controllers/templates/manifests.yaml
	sed -i.bak -e 's/namespace: system/namespace: "{{ .Values.namespace }}"/' helm/controllers/templates/manifests.yaml
	sed -i.bak -e 's/name: \(.*-webhook-configuration\)/name: korifi-controllers-\1/' helm/controllers/templates/manifests.yaml
	rm -f helm/controllers/templates/manifests.yaml.bak


manifests-job-task-runner:
	make -C job-task-runner manifests

manifests-kpack-image-builder:
	make -C kpack-image-builder manifests

manifests-statefulset-runner:
	make -C statefulset-runner manifests

generate: generate-controllers generate-job-task-runner generate-kpack-image-builder generate-statefulset-runner

generate-controllers: install-controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="controllers/hack/boilerplate.go.txt" paths="./controllers/..."

generate-job-task-runner:
	make -C job-task-runner generate

generate-kpack-image-builder:
	make -C kpack-image-builder generate

generate-statefulset-runner:
	make -C statefulset-runner generate

generate-fakes:
	go generate ./...

fmt: install-gofumpt install-shfmt
	$(GOFUMPT) -w .
	$(SHFMT) -w -i 2 -ci .

vet: ## Run go vet against code.
	go vet ./...

lint:
	golangci-lint run -v

test: lint test-controllers-api test-job-task-runner test-kpack-image-builder test-statefulset-runner test-e2e

test-controllers-api: test-api test-controllers

test-api: install-ginkgo fmt vet
	cd api && ../scripts/run-tests.sh --skip-package=test

test-controllers: install-ginkgo manifests-controllers generate-controllers fmt vet ## Run tests.
	cd controllers && GINKGO_NODES=$(CONTROLLERS_GINKGO_NODES) ../scripts/run-tests.sh

test-job-task-runner:
	make -C job-task-runner test

test-kpack-image-builder:
	make -C kpack-image-builder test

test-statefulset-runner:
	make -C statefulset-runner test

test-e2e: install-ginkgo
	./scripts/run-tests.sh tests/e2e

##@ Build

build: generate-controllers fmt vet ## Build manager binary.
	go build -o controllers/bin/manager controllers/main.go

run-api: fmt vet
	APICONFIG=$(shell pwd)/api/config/base/apiconfig go run ./api/main.go

run-controllers: manifests-controllers generate-controllers fmt vet ## Run a controller from your host.
	CONTROLLERSCONFIG=$(shell pwd)/controllers/config/base/controllersconfig ENABLE_WEBHOOKS=false go run ./controllers/main.go

run-job-task-runner:
	make -C job-task-runner run

run-kpack-image-builder:
	make -C kpack-image-builder run

run-statefulset-runner:
	make -C statefulset-runner run

docker-build: docker-build-api docker-build-controllers docker-build-job-task-runner docker-build-kpack-image-builder docker-build-statefulset-runner

docker-build-api:
	docker buildx build --load -f api/Dockerfile -t ${IMG_API} .

docker-build-api-debug:
	docker buildx build --load -f api/remote-debug/Dockerfile -t ${IMG_API} .

docker-build-controllers:
	docker buildx build --load -f controllers/Dockerfile -t ${IMG_CONTROLLERS} .

docker-build-controllers-debug:
	docker buildx build --load -f controllers/remote-debug/Dockerfile -t ${IMG_CONTROLLERS} .

docker-build-job-task-runner:
	make -C job-task-runner docker-build

docker-build-job-task-runner-debug:
	make -C job-task-runner docker-build-debug

docker-build-kpack-image-builder:
	make -C kpack-image-builder docker-build

docker-build-kpack-image-builder-debug:
	make -C kpack-image-builder docker-build-debug

docker-build-statefulset-runner:
	make -C statefulset-runner docker-build

docker-build-statefulset-runner-debug:
	make -C statefulset-runner docker-build-debug

docker-push: docker-push-api docker-push-controllers docker-push-job-task-runner docker-push-kpack-image-builder docker-push-statefulset-runner

docker-push-api:
	docker push ${IMG_API}

docker-push-controllers:
	docker push ${IMG_CONTROLLERS}

docker-push-job-task-runner:
	make -C job-task-runner docker-push

docker-push-kpack-image-builder:
	make -C kpack-image-builder docker-push

docker-push-statefulset-runner:
	make -C statefulset-runner docker-push

##@ Deployment

install-crds: manifests-controllers install-kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build controllers/config/crd | kubectl apply -f -

uninstall-crds: manifests-controllers install-kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build controllers/config/crd | kubectl delete -f -

deploy: deploy-controllers deploy-job-task-runner deploy-kpack-image-builder deploy-statefulset-runner deploy-api

deploy-kind: install-crds deploy-controllers deploy-job-task-runner deploy-kpack-image-builder deploy-statefulset-runner deploy-api-kind

deploy-kind-local: install-crds deploy-controllers-kind-local deploy-job-task-runner-kind-local deploy-kpack-image-builder-kind-local deploy-statefulset-runner deploy-api-kind-local


helm-api-values =

deploy-api: manifests-api
	helm upgrade --install api helm/api \
		--set=image=$(IMG_API) \
		$(helm-api-values) \
		--wait

APP_FQDN ?= vcap.me
kind-api-values = \
	--set=apiServer.url=localhost \
	--set=defaultDomainName=$(APP_FQDN)
local-registry = localregistry-docker-registry.default.svc.cluster.local:30050/kpack/packages

deploy-api-kind: helm-api-values = $(kind-api-values) \
	--set=packageRegistry.base=gcr.io/cf-relint-greengrass/korifi/kpack/beta
deploy-api-kind: deploy-api

deploy-api-kind-local: helm-api-values = $(kind-api-values) \
	--set=packageRegistry.base=$(local-registry)
deploy-api-kind-local: deploy-api

deploy-api-kind-local-debug: helm-api-values = $(kind-api-values) \
	--set=packageRegistry.base=$(local-registry) \
	--set=global.debug=true
deploy-api-kind-local-debug: deploy-api

helm-controllers-values =
deploy-controllers: manifests-controllers
	helm upgrade --install controllers helm/controllers \
		--set=image=$(IMG_CONTROLLERS) \
		$(helm-controllers-values) \
		--wait

kind-controllers-values = --set=taskTTL=5s
deploy-controllers-kind-local: helm-controllers-values = $(kind-controllers-values)
deploy-controllers-kind-local: deploy-controllers

deploy-controllers-kind-local-debug: helm-controllers-values = $(kind-controllers-values) \
	--set=global.debug=true
deploy-controllers-kind-local-debug: deploy-controllers

deploy-job-task-runner:
	make -C job-task-runner deploy

deploy-job-task-runner-kind-local:
	make -C job-task-runner deploy-kind-local

deploy-job-task-runner-kind-local-debug:
	make -C job-task-runner deploy-kind-local-debug

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

undeploy: undeploy-api undeploy-job-task-runner undeploy-kpack-image-builder undeploy-statefulset-runner undeploy-controllers

undeploy-api:
	@if helm status api 2>/dev/null; then \
		helm delete api --wait; \
	else \
		echo "api chart not found - skipping"; \
	fi

undeploy-controllers:
	@if helm status controllers 2>/dev/null; then \
		helm delete controllers --wait; \
	else \
		echo "controllers chart not found - skipping"; \
	fi

undeploy-job-task-runner:
	make -C job-task-runner undeploy

undeploy-kpack-image-builder:
	make -C kpack-image-builder undeploy

undeploy-statefulset-runner:
	make -C statefulset-runner undeploy

CONTROLLER_GEN = $(shell pwd)/controllers/bin/controller-gen
install-controller-gen: ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.9.2)

GOFUMPT = $(shell go env GOPATH)/bin/gofumpt
install-gofumpt:
	go install mvdan.cc/gofumpt@latest

SHFMT = $(shell go env GOPATH)/bin/shfmt
install-shfmt:
	go install mvdan.cc/sh/v3/cmd/shfmt@latest

install-ginkgo:
	go install github.com/onsi/ginkgo/v2/ginkgo

VENDIR = $(shell go env GOPATH)/bin/vendir
install-vendir:
	go install github.com/vmware-tanzu/carvel-vendir/cmd/vendir@latest

vendir-update-dependencies: install-vendir
	$(VENDIR) sync --chdir tests

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
