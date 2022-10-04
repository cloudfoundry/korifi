
# Image URL to use all building/pushing image targets
IMG_SSR ?= cloudfoundry/korifi-statefulset-runner:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.23
CLUSTER_NAME ?= "e2e"

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

.PHONY: all
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

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: install-controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) \
		paths="./..." \
		rbac:roleName=korifi-statefulset-runner-appworkload-manager-role \
		webhook \
		output:rbac:artifacts:config=../helm/statefulset-runner/templates \
		output:webhook:artifacts:config=../helm/statefulset-runner/templates

	sed -i.bak -e '/^metadata:.*/a \ \ annotations:\n    cert-manager.io/inject-ca-from: "{{ .Values.namespace }}/korifi-statefulset-runner-serving-cert"' ../helm/statefulset-runner/templates/manifests.yaml
	sed -i.bak -e 's/name: \(webhook-service\)/name: korifi-statefulset-runner-\1/' ../helm/statefulset-runner/templates/manifests.yaml
	sed -i.bak -e 's/namespace: system/namespace: "{{ .Values.namespace }}"/' ../helm/statefulset-runner/templates/manifests.yaml
	sed -i.bak -e 's/name: \(.*-webhook-configuration\)/name: korifi-statefulset-runner-\1/' ../helm/statefulset-runner/templates/manifests.yaml
	sed -i.bak -e '/failurePolicy:.*/a\  objectSelector:\n    matchLabels:\n      korifi.cloudfoundry.org/add-stsr-index: "true"' ../helm/statefulset-runner/templates/manifests.yaml
	rm -f ../helm/statefulset-runner/templates/manifests.yaml.bak


.PHONY: generate
generate: install-controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: generate-fakes
generate-fakes:
	go generate ./...

.PHONY: fmt
fmt: install-gofumpt install-shfmt
	$(GOFUMPT) -w .
	$(SHFMT) -w -i 2 -ci .

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: install-ginkgo manifests generate fmt vet ## Run tests.
	../scripts/run-tests.sh

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary.
	go build -o bin/manager main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	ENABLE_WEBHOOKS=false go run ./main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	docker buildx build --load -f ./Dockerfile -t ${IMG_SSR} ..

.PHONY: docker-build-debug
docker-build-debug: ## Build docker image with the manager.
	docker buildx build --load -f ./remote-debug/Dockerfile -t ${IMG_SSR} ..

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG_SSR}

kind-load-image:
	kind load docker-image --name ${CLUSTER_NAME} ${IMG_SSR}

##@ Deployment

helm-values =
deploy: manifests
	helm upgrade --install statefulset-runner ../helm/statefulset-runner \
		--set=image=$(IMG_SSR) \
		$(helm-values) \
		--wait

deploy-kind-local: deploy

deploy-kind-local-debug: helm-values = --set=global.debug=true
deploy-kind-local-debug: deploy

.PHONY: undeploy
undeploy:
	@if helm status statefulset-runner 2>/dev/null; then \
		helm delete statefulset-runner --wait; \
	else \
		echo "statefulset-runner chart not found - skipping"; \
	fi

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
.PHONY: install-controller-gen
install-controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.9.2)


GOFUMPT = $(shell go env GOPATH)/bin/gofumpt
install-gofumpt:
	go install mvdan.cc/gofumpt@latest

SHFMT = $(shell go env GOPATH)/bin/shfmt
install-shfmt:
	go install mvdan.cc/sh/v3/cmd/shfmt@latest

install-ginkgo:
	go install github.com/onsi/ginkgo/v2/ginkgo

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
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
