#!/usr/bin/env bash

set -euxo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$SCRIPT_DIR/.."
API_DIR="$ROOT_DIR/api"
CTRL_DIR="$ROOT_DIR/controllers"
EIRINI_CONTROLLER_DIR="$ROOT_DIR/../eirini-controller"
export PATH="$PATH:$API_DIR/bin"

ensure_kind_cluster() {
  if ! kind get clusters | grep -q "$cluster"; then
    current_cluster="$(kubectl config current-context)" || true
    cat <<EOF | kind create cluster --name "$cluster" --wait 5m --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
EOF
    if [[ -n "$current_cluster" ]]; then
      kubectl config use-context "$current_cluster"
    fi
  fi
  kind export kubeconfig --name "$cluster" --kubeconfig "$HOME/.kube/$cluster.yml"
}

deploy_cf_k8s_controllers() {
  pushd $ROOT_DIR > /dev/null
  {
    "$SCRIPT_DIR/install-dependencies.sh"
    export KUBEBUILDER_ASSETS=$ROOT_DIR/testbin/bin
    echo $PWD
    make generate-controllers
    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      export IMG_CONTROLLERS=${CONTROLLERS_IMG:-"cf-k8s-controllers:$(uuidgen)"}
      make docker-build-controllers
      kind load docker-image --name "$cluster" "$IMG_CONTROLLERS"
    fi
    make install-crds
    make deploy-controllers
  }
  popd > /dev/null
}

deploy_cf_k8s_api() {
  pushd $ROOT_DIR > /dev/null
  {
    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      export IMG_API=${API_IMG:-"cf-k8s-api:$(uuidgen)"}
      make docker-build-api
      kind load docker-image --name "$cluster" "$IMG_API"
    fi
    make deploy-api-kind-auth
  }
  popd > /dev/null
}

cluster=${1:?specify cluster name}
ensure_kind_cluster "$cluster"
export KUBECONFIG="$HOME/.kube/$cluster.yml"
deploy_cf_k8s_controllers
deploy_cf_k8s_api
