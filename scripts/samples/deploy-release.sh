#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT_DIR="${ROOT_DIR}/scripts"
API_DIR="${ROOT_DIR}/api"
CONTROLLER_DIR="${ROOT_DIR}/controllers"
export PATH="${PATH}:${API_DIR}/bin"

release_dir="$(mktemp -d)"
trap "rm -rf $release_dir" EXIT

source "$SCRIPT_DIR/common.sh"

cluster="korifi"

function unpack_release() {
  release_archive="${1:-}"
  if [[ ! -f $release_archive ]]; then
    echo "Cannot find release archive '$release_archive', aborting"
    exit 1
  fi
  tar xzf "$1" -C "$release_dir"
}

function ensure_kind_cluster() {
  if ! kind get clusters | grep -q "${cluster}"; then
    cat <<EOF | kind create cluster --name "${cluster}" --wait 5m --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
EOF
  fi

  kind export kubeconfig --name "${cluster}"
}

function ensure_local_registry() {
  helm repo add twuni https://helm.twun.io
  helm upgrade --install localregistry twuni/docker-registry --set service.type=NodePort,service.nodePort=30050,service.port=30050

  # reconfigure containerd to allow insecure connection to our local registry on localhost
  docker cp "${cluster}-control-plane:/etc/containerd/config.toml" /tmp/config.toml
  if ! grep -q localregistry-docker-registry\.default\.svc\.cluster\.local /tmp/config.toml; then
    cat <<EOF >>/tmp/config.toml

[plugins."io.containerd.grpc.v1.cri".registry]
  [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localregistry-docker-registry.default.svc.cluster.local:30050"]
      endpoint = ["http://127.0.0.1:30050"]
  [plugins."io.containerd.grpc.v1.cri".registry.configs]
    [plugins."io.containerd.grpc.v1.cri".registry.configs."127.0.0.1:30050".tls]
      insecure_skip_verify = true
EOF
    docker cp /tmp/config.toml "${cluster}-control-plane:/etc/containerd/config.toml"
    docker exec "${cluster}-control-plane" bash -c "systemctl restart containerd"
    echo "waiting for containerd to restart..."
    sleep 10
  fi
}

function install_dependencies() {
  pushd "${ROOT_DIR}" >/dev/null
  {
    export DOCKER_SERVER="localregistry-docker-registry.default.svc.cluster.local:30050"
    export DOCKER_USERNAME="whatevs"
    export DOCKER_PASSWORD="whatevs"
    export KPACK_TAG="localregistry-docker-registry.default.svc.cluster.local:30050/cf-relint-greengrass/korifi/kpack/beta"

    "${SCRIPT_DIR}/install-dependencies.sh"
  }
  popd >/dev/null
}

function deploy_korifi() {
  kubectl apply -f "$release_dir" --recursive
  create_tls_cert "korifi-workloads-ingress-cert" "korifi-controllers" "\*.vcap.me"
  create_tls_cert "korifi-api-ingress-cert" "korifi-api" "api.vcap.me"

  kubectl rollout status deployment/korifi-controllers-controller-manager -w -n korifi-controllers-system
  kubectl rollout status deployment/korifi-api-deployment -w -n korifi-api-system
  kubectl rollout status deployment/korifi-job-task-runner-controller-manager -w -n korifi-job-task-runner-system
  kubectl rollout status deployment/korifi-kpack-build-controller-manager -w -n korifi-kpack-build-system
  kubectl rollout status deployment/korifi-statefulset-runner-controller-manager -w -n korifi-statefulset-runner-system

  kubectl apply -f ${CONTROLLER_DIR}/config/samples/cfdomain.yaml
}

unpack_release "$@"
ensure_kind_cluster "${cluster}"
ensure_local_registry
install_dependencies
deploy_korifi
