#!/usr/bin/env bash

set -euxo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$SCRIPT_DIR/.."
API_DIR="$ROOT_DIR/api"
CTRL_DIR="$ROOT_DIR/controllers"
EIRINI_CONTROLLER_DIR="$ROOT_DIR/../eirini-controller"
export PATH="$PATH:$API_DIR/bin"

# undo *_IMG changes in config and reference
clean_up_img_refs() {
  cd "$ROOT_DIR"
  unset IMG_CONTROLLERS
  unset IMG_API
  make build-reference
}
trap clean_up_img_refs EXIT

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

ensure_local_registry() {
  helm upgrade --install localregistry twuni/docker-registry --set service.type=NodePort,service.nodePort=30050,service.port=30050

  # TODO-maybe we don't need to add the /etc/hosts hack if we configure the mirror below to redirect to 127.0.0.1?
  docker exec "${cluster}-control-plane" bash -c 'echo "127.0.0.1 localregistry-docker-registry.default.svc.cluster.local" >> /etc/hosts'

  # reconfigure containerd to allow insecure connection to our local registry
  docker cp ${cluster}-control-plane:/etc/containerd/config.toml /tmp/config.toml
  if ! grep -q localregistry-docker-registry\.default\.svc\.cluster\.local /tmp/config.toml; then
    cat <<EOF >> /tmp/config.toml

[plugins."io.containerd.grpc.v1.cri".registry]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localregistry-docker-registry.default.svc.cluster.local:30050"]
      endpoint = ["http://localregistry-docker-registry.default.svc.cluster.local:30050"]
  [plugins."io.containerd.grpc.v1.cri".registry.configs]
    [plugins."io.containerd.grpc.v1.cri".registry.configs."localregistry-docker-registry.default.svc.cluster.local:30050".tls]
      insecure_skip_verify = true
EOF
    docker cp /tmp/config.toml ${cluster}-control-plane:/etc/containerd/config.toml
    docker exec "${cluster}-control-plane" bash -c "systemctl restart containerd"
    echo "waiting for containerd to restart..."
    sleep 30
  fi
}

deploy_cf_k8s_controllers() {
  pushd $ROOT_DIR >/dev/null
  {
    if [ -n "$use_local_registry" ]; then
      export DOCKER_SERVER="localregistry-docker-registry.default.svc.cluster.local:30050"
      export DOCKER_USERNAME="whatevs"
      export DOCKER_PASSWORD="whatevs"
    fi

    "$SCRIPT_DIR/install-dependencies.sh"
    export KUBEBUILDER_ASSETS=$ROOT_DIR/testbin/bin
    echo $PWD
    make generate-controllers
    IMG_CONTROLLERS=${IMG_CONTROLLERS:-"cf-k8s-controllers:$(uuidgen)"}
    export IMG_CONTROLLERS
    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      make docker-build-controllers
    fi
    kind load docker-image --name "$cluster" "$IMG_CONTROLLERS"
    make install-crds
    if [ -n "$use_local_registry" ]; then
      make deploy-controllers-kind-local
    else
      make deploy-controllers-kind
    fi
  }
  popd >/dev/null
}

deploy_cf_k8s_api() {
  pushd $ROOT_DIR >/dev/null
  {
    IMG_API=${IMG_API:-"cf-k8s-api:$(uuidgen)"}
    export IMG_API
    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      make docker-build-api
    fi
    kind load docker-image --name "$cluster" "$IMG_API"
    if [ -n "$use_local_registry" ]; then
      make deploy-api-kind-local
    else
      make deploy-api-kind-auth
    fi
  }
  popd >/dev/null
}

cluster=${1:?specify cluster name}
ensure_kind_cluster "$cluster"
use_local_registry=${2}
if [ -n "$use_local_registry" ]; then
  ensure_local_registry
fi
export KUBECONFIG="$HOME/.kube/$cluster.yml"
deploy_cf_k8s_controllers
deploy_cf_k8s_api
