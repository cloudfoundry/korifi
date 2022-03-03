#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$SCRIPT_DIR/.."
API_DIR="$ROOT_DIR/api"
CTRL_DIR="$ROOT_DIR/controllers"
EIRINI_CONTROLLER_DIR="$ROOT_DIR/../eirini-controller"
export PATH="$PATH:$API_DIR/bin"

function usage_text() {
  cat <<EOF
Usage:
  $(basename "$0") <kind cluster name>

flags:
  -l, --use-local-registry
      Deploys a local container registry to the kind cluster.

  -v, --verbose
      Verbose output (bash -x).

  -c, --controllers-only
      Skips all steps except for building and installing
      controllers. (This will fail unless the script is
      being re-run.)

  -a, --api-only
      Skips all steps except for building and installing
      the API shim. (This will fail unless the script is
      being re-run.)

EOF
  exit 1
}

cluster=""
use_local_registry=""
controllers_only=""
api_only=""
while [[ $# -gt 0 ]]; do
  i=$1
  case $i in
  -l | --use-local-registry)
    use_local_registry="true"
    shift
    ;;
  -c | --controllers-only)
    controllers_only="true"
    shift
    ;;
  -a | --api-only)
    api_only="true"
    shift
    ;;
  -v | --verbose)
    set -x
    shift
    ;;
  -h | --help | help)
    usage_text >&2
    exit 0
    ;;
  *)
    if [[ -n "$cluster" ]]; then
      echo -e "Error: Unexpected argument: ${i/=*/}\n" >&2
      usage_text >&2
      exit 1
    fi
    cluster=$1
    shift
    ;;
  esac
done

if [[ -z "$cluster" ]]; then
  echo -e "Error: missing argument <kind cluster name>" >&2
  usage_text >&2
  exit 1
fi

# undo *_IMG changes in config and reference
clean_up_img_refs() {
  cd "$ROOT_DIR"
  unset IMG_CONTROLLERS
  unset IMG_API
  make build-reference
}
trap clean_up_img_refs EXIT

ensure_kind_cluster() {
  if [[ -n "$controllers_only" ]]; then return 0; fi
  if [[ -n "$api_only" ]]; then return 0; fi

  if ! kind get clusters | grep -q "$cluster"; then
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
  - containerPort: 30050
    hostPort: 30050
    protocol: TCP
EOF
  fi

  kind export kubeconfig --name "$cluster"
}

ensure_local_registry() {
  if [[ -z "$use_local_registry" ]]; then return 0; fi
  if [[ -n "$controllers_only" ]]; then return 0; fi
  if [[ -n "$api_only" ]]; then return 0; fi

  helm repo add twuni https://helm.twun.io
  helm upgrade --install localregistry twuni/docker-registry --set service.type=NodePort,service.nodePort=30050,service.port=30050

  # reconfigure containerd to allow insecure connection to our local registry on localhost
  docker cp ${cluster}-control-plane:/etc/containerd/config.toml /tmp/config.toml
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
    docker cp /tmp/config.toml ${cluster}-control-plane:/etc/containerd/config.toml
    docker exec "${cluster}-control-plane" bash -c "systemctl restart containerd"
    echo "waiting for containerd to restart..."
    sleep 10
  fi
}

install_dependencies() {
  if [[ -n "$controllers_only" ]]; then return 0; fi
  if [[ -n "$api_only" ]]; then return 0; fi

  pushd $ROOT_DIR >/dev/null
  {
    if [[ -n "$use_local_registry" ]]; then
      export DOCKER_SERVER="localregistry-docker-registry.default.svc.cluster.local:30050"
      export DOCKER_USERNAME="whatevs"
      export DOCKER_PASSWORD="whatevs"
    fi

    "$SCRIPT_DIR/install-dependencies.sh"

    # install metrics server only on local cluster
    DEP_DIR="$(cd "${SCRIPT_DIR}/../dependencies" && pwd)"
    echo "*********************************************"
    echo "Installing metrics-server"
    echo "*********************************************"
    kubectl apply -f "${DEP_DIR}/metrics-server-local-0.6.1.yaml"

  }
  popd >/dev/null
}

deploy_cf_k8s_controllers() {
  if [[ -n "$api_only" ]]; then return 0; fi

  pushd $ROOT_DIR >/dev/null
  {
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
    if [[ -n "$use_local_registry" ]]; then
      make deploy-controllers-kind-local
    else
      make deploy-controllers-kind
    fi
  }
  popd >/dev/null

  # note: we may want to make the default domain configurable. For now it is "vcap.me"
  kubectl apply -f ${CTRL_DIR}/config/samples/cfdomain.yaml
}

deploy_cf_k8s_api() {
  if [[ -n "$controllers_only" ]]; then return 0; fi

  pushd $ROOT_DIR >/dev/null
  {
    IMG_API=${IMG_API:-"cf-k8s-api:$(uuidgen)"}
    export IMG_API
    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      make docker-build-api
    fi
    kind load docker-image --name "$cluster" "$IMG_API"

    if [[ -n "$use_local_registry" ]]; then
      make deploy-api-kind-local
    else
      make deploy-api-kind-auth
    fi

    openssl req -x509 -newkey rsa:4096 -keyout /tmp/api-tls.key -out /tmp/api-tls.crt -nodes -subj '/CN=localhost' -addext "subjectAltName = DNS:*.vcap.me" -days 365
    kubectl create secret tls   cf-k8s-api-ingress-cert   --cert=/tmp/api-tls.crt --key=/tmp/api-tls.key -n cf-k8s-api-system

  }
  popd >/dev/null
}

ensure_kind_cluster "$cluster"
ensure_local_registry
install_dependencies
deploy_cf_k8s_controllers
deploy_cf_k8s_api
