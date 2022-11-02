
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

webhooks-file = ../helm/statefulset-runner/templates/manifests.yaml
.PHONY: manifests
manifests: install-controller-gen install-yq
	$(CONTROLLER_GEN) \
		paths="./..." \
		rbac:roleName=korifi-statefulset-runner-appworkload-manager-role \
		webhook \
		output:rbac:artifacts:config=../helm/statefulset-runner/templates \
		output:webhook:artifacts:config=../helm/statefulset-runner/templates

	$(YQ) -i 'with(.metadata; .annotations["cert-manager.io/inject-ca-from"]="{{ .Release.Namespace }}/korifi-statefulset-runner-serving-cert")' $(webhooks-file)
	$(YQ) -i 'with(.metadata; .name="korifi-statefulset-runner-" + .name)' $(webhooks-file)
	$(YQ) -i 'with(.webhooks[]; .clientConfig.service.namespace="{{ .Release.Namespace }}")' $(webhooks-file)
	$(YQ) -i 'with(.webhooks[]; .clientConfig.service.name="korifi-statefulset-runner-" + .clientConfig.service.name)' $(webhooks-file)
	$(YQ) -i 'with(.webhooks[]; .objectSelector.matchLabels["korifi.cloudfoundry.org/add-stsr-index"]="true")' $(webhooks-file)


.PHONY: generate
generate: install-controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: test
test: install-ginkgo manifests generate ## Run tests.
	../scripts/run-tests.sh

.PHONY: install-controller-gen
CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
install-controller-gen:
	GOBIN=$(shell pwd)/bin go install sigs.k8s.io/controller-tools/cmd/controller-gen

install-ginkgo:
	go install github.com/onsi/ginkgo/v2/ginkgo

YQ = $(shell pwd)/bin/yq
install-yq:
	GOBIN=$(shell pwd)/bin go install github.com/mikefarah/yq/v4@latest
