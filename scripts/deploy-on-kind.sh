#!/bin/bash

set -euxo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
API_DIR="$SCRIPT_DIR/.."
CTRL_DIR="$API_DIR/../cf-k8s-controllers"
EIRINI_CONTROLLER_DIR="$API_DIR/../eirini-controller"
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

deploy_eirini_controller() {
  if ! command -v kbld >/dev/null; then
    curl -L https://carvel.dev/install.sh | K14SIO_INSTALL_BIN_DIR=$API_DIR/bin bash
  fi

  pushd "$EIRINI_CONTROLLER_DIR"
  {
    "./deployment/scripts/generate-secrets.sh" "*.eirini-controller.svc"

    render_dir=$(mktemp -d)
    trap "rm -rf $render_dir" EXIT
    webhooks_ca_bundle="$(kubectl get secret -n eirini-controller eirini-webhooks-certs -o jsonpath="{.data['tls\.ca']}")"
    kbld -f "$SCRIPT_DIR/assets/eirini-controller-kbld.yml" \
      -f "$EIRINI_CONTROLLER_DIR/deployment/helm/values-template.yaml" \
      >"$EIRINI_CONTROLLER_DIR/deployment/helm/values.yaml"

    "$EIRINI_CONTROLLER_DIR/deployment/scripts/render-templates.sh" eirini-controller "$render_dir" \
      --values "$EIRINI_CONTROLLER_DIR/deployment/scripts/assets/value-overrides.yaml" \
      --set "webhooks.ca_bundle=$webhooks_ca_bundle" \
      --set "workloads.create_namespaces=true" \
      --set "workloads.default_namespace=cf"
    for img in $(grep -oh "kbld:.*" "$EIRINI_CONTROLLER_DIR/deployment/helm/values.yaml"); do
      kind load docker-image --name "$cluster" "$img"
    done
    kapp -y delete -a eirini-controller
    kapp -y deploy -a eirini-controller -f "$render_dir/templates/"
  }
}

cluster=${1:?specify cluster name}
ensure_kind_cluster "$cluster"
export KUBECONFIG="$HOME/.kube/$cluster.yml"
deploy_cf_k8s_controllers
deploy_cf_k8s_api
deploy_eirini_controller
