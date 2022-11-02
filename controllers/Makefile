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

# Use gsed on Mac, sed on linux
ifeq (,$(shell which gsed))
SED=sed
else
SED=gsed
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

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

manifests: install-controller-gen
	$(CONTROLLER_GEN) \
		paths="./..." \
		crd \
		rbac:roleName=korifi-controllers-manager-role \
		webhook \
		output:crd:artifacts:config=../helm/controllers/templates/crds \
		output:rbac:artifacts:config=../helm/controllers/templates \
		output:webhook:artifacts:config=../helm/controllers/templates

	$(SED) -i.bak -e '/^metadata:.*/a \ \ annotations:\n    cert-manager.io/inject-ca-from: "{{ .Release.Namespace }}/korifi-controllers-serving-cert"' ../helm/controllers/templates/manifests.yaml
	$(SED) -i.bak -e 's/name: \(webhook-service\)/name: korifi-controllers-\1/' ../helm/controllers/templates/manifests.yaml
	$(SED) -i.bak -e 's/namespace: system/namespace: "{{ .Release.Namespace }}"/' ../helm/controllers/templates/manifests.yaml
	$(SED) -i.bak -e 's/name: \(.*-webhook-configuration\)/name: korifi-controllers-\1/' ../helm/controllers/templates/manifests.yaml
	rm -f ../helm/controllers/templates/manifests.yaml.bak

generate: install-controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./controllers/..."

test: install-ginkgo manifests generate ## Run tests.
	GINKGO_NODES=$(CONTROLLERS_GINKGO_NODES) ../scripts/run-tests.sh

CONTROLLER_GEN = $(shell pwd)/controllers/bin/controller-gen
install-controller-gen: ## Download controller-gen locally if necessary.
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.9.2)

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
