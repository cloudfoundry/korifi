SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development
export GOBIN = $(shell pwd)/bin
export PATH := $(shell pwd)/bin:$(PATH)

.PHONY: manifests
manifests: bin/controller-gen
	controller-gen \
		paths="./..." \
		rbac:roleName=korifi-knative-runner-appworkload-manager-role \
		output:rbac:artifacts:config=../helm/korifi/knative-runner

.PHONY: generate
generate:
	go generate ./...

.PHONY: test
test: ## Run knative-runner unit tests.
	../scripts/run-tests.sh

##@ Build Dependencies
bin:
	mkdir -p bin

bin/controller-gen: bin
	go install sigs.k8s.io/controller-tools/cmd/controller-gen
