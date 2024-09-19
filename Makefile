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
export GOBIN = $(shell pwd)/bin
export PATH := $(shell pwd)/bin:$(PATH)

CONTROLLERS=controllers job-task-runner kpack-image-builder statefulset-runner
COMPONENTS=api $(CONTROLLERS)

manifests: bin/controller-gen
	controller-gen \
		paths="./model/..." \
		crd \
		output:crd:artifacts:config=helm/korifi/controllers/crds
	@for comp in $(COMPONENTS); do make -C $$comp manifests; done

generate: bin/controller-gen
	controller-gen object:headerFile="controllers/hack/boilerplate.go.txt" paths="./model/..."
	@for comp in $(CONTROLLERS); do make -C $$comp generate; done
	go run ./scripts/helmdoc/main.go > README.helm.md

bin/controller-gen: bin
	go install sigs.k8s.io/controller-tools/cmd/controller-gen

generate-fakes:
	go generate ./...

fmt: bin/gofumpt bin/shfmt
	gofumpt -w .
	shfmt -f . | grep -v '^tests/vendor' | xargs shfmt -w -i 2 -ci

vet: ## Run go vet against code.
	go vet ./...

lint: fmt vet gosec staticcheck golangci-lint

gosec: bin/gosec
	gosec --exclude=G101,G304,G401,G404,G505 --exclude-dir=tests ./...

staticcheck: bin/staticcheck
	staticcheck ./...

golangci-lint: bin/golangci-lint
	golangci-lint run

test: lint
	@for comp in $(COMPONENTS); do make -C $$comp test; done
	make test-tools
	make test-e2e

test-tools:
	./scripts/run-tests.sh tools

test-e2e: build-dorifi
	./scripts/run-tests.sh tests/e2e

test-crds: build-dorifi
	./scripts/run-tests.sh tests/crds

test-smoke: build-dorifi bin/cf
	./scripts/run-tests.sh tests/smoke


build-dorifi:
	CGO_ENABLED=0 GOOS=linux go build -C tests/assets/dorifi-golang -o ../dorifi/dorifi .
	CGO_ENABLED=0 GOOS=linux go build -C tests/assets/dorifi-golang -o ../multi-process/dorifi .
	CGO_ENABLED=0 GOOS=linux go build -C tests/assets/sample-broker-golang -o ../sample-broker/sample-broker .

bin:
	mkdir -p bin

bin/gofumpt:
	go install mvdan.cc/gofumpt@latest

bin/shfmt:
	go install mvdan.cc/sh/v3/cmd/shfmt@latest

bin/vendir:
	go install carvel.dev/vendir/cmd/vendir@latest

bin/gosec:
	go install github.com/securego/gosec/v2/cmd/gosec@latest

bin/staticcheck:
	go install honnef.co/go/tools/cmd/staticcheck@latest

bin/golangci-lint:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

bin/cf:
	mkdir -p $(GOBIN)
	curl -fsSL "https://packages.cloudfoundry.org/stable?release=linux64-binary&version=v8&source=github" \
	  | tar -zx cf8 \
	  && mv cf8 $(GOBIN)/cf \
	  && chmod +x $(GOBIN)/cf

bin/yq: bin
	go install github.com/mikefarah/yq/v4@latest

bin/setup-envtest: bin
	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

vendir-update-dependencies: bin/vendir
	vendir sync --chdir tests
