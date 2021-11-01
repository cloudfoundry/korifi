#!/bin/bash

set -euxo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
API_DIR="$SCRIPT_DIR/.."
CTRL_DIR="$API_DIR/../controllers"
EIRINI_CONTROLLER_DIR="$API_DIR/../../eirini-controller"
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
  pushd "$CTRL_DIR"
  {
    ./hack/install-dependencies.sh
    local uuid
    uuid="$(uuidgen)"
    export IMG="cf-k8s-controllers:$uuid"
    export KUBEBUILDER_ASSETS=$CTRL_DIR/testbin/bin
    make generate
    make docker-build
    kind load docker-image --name "$cluster" "$IMG"
    make install
    make deploy
  }
  popd
}

deploy_cf_k8s_api() {
  pushd "$API_DIR"
  {
    local uuid
    uuid="$(uuidgen)"
    export IMG="cf-k8s-api:$uuid"
    make docker-build
    kind load docker-image --name "$cluster" "$IMG"
    make deploy-kind
  }
  popd
}

cluster=${1:?specify cluster name}
ensure_kind_cluster "$cluster"
export KUBECONFIG="$HOME/.kube/$cluster.yml"
deploy_cf_k8s_controllers
deploy_cf_k8s_api
