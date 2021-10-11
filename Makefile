# Image URL to use all building/pushing image targets
IMG ?= cloudfoundry/cf-k8s-api:latest

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

.DEFAULT_GOAL := test

.PHONY: hnc-install test test-e2e kustomize docker-build fmt vet

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
test: fmt vet ## Run tests.
	mkdir -p ${ENVTEST_ASSETS_DIR}
	test -f ${ENVTEST_ASSETS_DIR}/setup-envtest.sh || curl -sSLo ${ENVTEST_ASSETS_DIR}/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.8.3/hack/setup-envtest.sh
	source ${ENVTEST_ASSETS_DIR}/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test ./... -coverprofile cover.out -shuffle on

test-e2e: ginkgo
ifndef SKIP_DEPLOY
	./scripts/deploy-on-kind.sh e2e
endif
	KUBECONFIG="${HOME}/.kube/e2e.yml" API_SERVER_ROOT=http://localhost ROOT_NAMESPACE=cf-k8s-api-system $(GINKGO) -p -randomizeAllSpecs -randomizeSuites -keepGoing -slowSpecThreshold 30 -tags e2e tests/e2e

run: fmt vet ## Run a controller from your host.
	go run ./main.go

manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=cf-admin-clusterrole paths=./... output:rbac:artifacts:config=config/base/rbac

generate: fmt vet
	go generate ./...

deploy: kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/base && $(KUSTOMIZE) edit set image cloudfoundry/cf-k8s-api=${IMG}
	$(KUSTOMIZE) build config/base | kubectl apply -f -

deploy-kind: kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/base && $(KUSTOMIZE) edit set image cloudfoundry/cf-k8s-api=${IMG}
	$(KUSTOMIZE) build config/overlays/kind | kubectl apply -f -

undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/base | kubectl delete -f -

docker-build: ## Build docker image with the manager.
	docker build -t ${IMG} .

build-reference: kustomize
	cd config/base && $(KUSTOMIZE) edit set image cloudfoundry/cf-k8s-api=${IMG}
	$(KUSTOMIZE) build config/base -o reference/cf-k8s-api.yaml

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.5.0)

KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.2.0)

GINKGO = $(shell pwd)/bin/ginkgo
ginkgo:
	$(call go-get-tool,$(GINKGO),github.com/onsi/ginkgo/ginkgo@latest)

hnc-install:
	./scripts/install-hnc.sh

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef
