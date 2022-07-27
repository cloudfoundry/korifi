#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT_DIR="${ROOT_DIR}/scripts"
API_DIR="${ROOT_DIR}/api"
CONTROLLER_DIR="${ROOT_DIR}/controllers"
export PATH="${PATH}:${API_DIR}/bin"

source "$SCRIPT_DIR/common.sh"

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

  -d, --default-domain
      Creates the vcap.me CF domain.

  -D, --debug
      Builds controller and api images with debugging hooks and
      wires up localhost:30051 (controller) and localhost:30052 (api) for remote debugging.
EOF
  exit 1
}

cluster=""
use_local_registry=""
controllers_only=""
api_only=""
default_domain=""
debug=""
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
    -d | --default-domain)
      default_domain="true"
      shift
      ;;
    -D | --debug)
      debug="true"
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
      if [[ -n "${cluster}" ]]; then
        echo -e "Error: Unexpected argument: ${i/=*/}\n" >&2
        usage_text >&2
        exit 1
      fi
      cluster=$1
      shift
      ;;
  esac
done

if [[ -z "${cluster}" ]]; then
  echo -e "Error: missing argument <kind cluster name>" >&2
  usage_text >&2
  exit 1
fi

if [[ -n "${debug}" ]]; then
  if [[ -z "${use_local_registry}" ]]; then
    echo -e "Error: currently debugging requires local registry (only because Kustomize is hard, not for real reasons)" >&2
    exit 1
  fi
fi

# undo *_IMG changes in config and reference
function clean_up_img_refs() {
  cd "${ROOT_DIR}"
  unset IMG_CONTROLLERS
  unset IMG_API
  make set-image-ref

  cd "${ROOT_DIR}/kpack-image-builder"
  unset IMG_KIB
  make set-image-ref

  cd "${ROOT_DIR}/statefulset-runner"
  unset IMG_SSR
  make set-image-ref
}

function ensure_kind_cluster() {
  if [[ -n "${controllers_only}" ]]; then return 0; fi
  if [[ -n "${api_only}" ]]; then return 0; fi

  if ! kind get clusters | grep -q "${cluster}"; then
    cat <<EOF | kind create cluster --name "${cluster}" --wait 5m --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
containerdConfigPatches:
- |-
  [plugins."io.containerd.grpc.v1.cri".registry]
    [plugins."io.containerd.grpc.v1.cri".registry.mirrors]
      [plugins."io.containerd.grpc.v1.cri".registry.mirrors."localregistry-docker-registry.default.svc.cluster.local:30050"]
        endpoint = ["http://127.0.0.1:30050"]
    [plugins."io.containerd.grpc.v1.cri".registry.configs]
      [plugins."io.containerd.grpc.v1.cri".registry.configs."127.0.0.1:30050".tls]
        insecure_skip_verify = true
featureGates:
  EphemeralContainers: true
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
  - containerPort: 30051
    hostPort: 30051
    protocol: TCP
  - containerPort: 30052
    hostPort: 30052
    protocol: TCP
EOF
  fi

  kind export kubeconfig --name "${cluster}"
}

function ensure_local_registry() {
  if [[ -z "${use_local_registry}" ]]; then return 0; fi
  if [[ -n "${controllers_only}" ]]; then return 0; fi
  if [[ -n "${api_only}" ]]; then return 0; fi

  helm repo add twuni https://helm.twun.io
  helm upgrade --install localregistry twuni/docker-registry \
    --set service.type=NodePort,service.nodePort=30050,service.port=30050 \
    --set persistence.enabled=true
}

function install_dependencies() {
  if [[ -n "${controllers_only}" ]]; then return 0; fi
  if [[ -n "${api_only}" ]]; then return 0; fi

  pushd "${ROOT_DIR}" >/dev/null
  {
    if [[ -n "${use_local_registry}" ]]; then
      export DOCKER_SERVER="localregistry-docker-registry.default.svc.cluster.local:30050"
      export DOCKER_USERNAME="whatevs"
      export DOCKER_PASSWORD="whatevs"
      export KPACK_TAG="localregistry-docker-registry.default.svc.cluster.local:30050/cf-relint-greengrass/korifi/kpack/beta"
    fi

    "${SCRIPT_DIR}/install-dependencies.sh"

    # install metrics server only on local cluster
    DEP_DIR="$(cd "${SCRIPT_DIR}/../dependencies" && pwd)"
    echo "*********************************************"
    echo "Installing metrics-server"
    echo "*********************************************"
    kubectl apply -f "${DEP_DIR}/metrics-server-local-0.6.1.yaml"

  }
  popd >/dev/null
}

function deploy_korifi_controllers() {
  if [[ -n "${api_only}" ]]; then return 0; fi

  pushd "${ROOT_DIR}" >/dev/null
  {
    export KUBEBUILDER_ASSETS="${ROOT_DIR}/testbin/bin"
    echo "${PWD}"
    make generate-controllers
    IMG_CONTROLLERS=${IMG_CONTROLLERS:-"korifi-controllers:$(uuidgen)"}
    export IMG_CONTROLLERS
    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      if [[ -z "${debug}" ]]; then
        make docker-build-controllers
      else
        make docker-build-controllers-debug
      fi
    fi
    kind load docker-image --name "${cluster}" "${IMG_CONTROLLERS}"

    make install-crds
    if [[ -n "${use_local_registry}" ]]; then
      if [[ -z "${debug}" ]]; then
        make deploy-controllers-kind-local
      else
        make deploy-controllers-kind-local-debug
      fi
    else
      make deploy-controllers
    fi

    create_tls_secret "korifi-workloads-ingress-cert" "korifi-controllers-system" "*.vcap.me"
  }
  popd >/dev/null

  kubectl rollout status deployment/korifi-controllers-controller-manager -w -n korifi-controllers-system

  if [[ -n "${default_domain}" ]]; then
    sed 's/vcap\.me/'${APP_FQDN:-vcap.me}'/' ${CONTROLLER_DIR}/config/samples/cfdomain.yaml | kubectl apply -f-
  fi
}

function deploy_korifi_api() {
  if [[ -n "${controllers_only}" ]]; then return 0; fi

  pushd "${ROOT_DIR}" >/dev/null
  {
    IMG_API=${IMG_API:-"korifi-api:$(uuidgen)"}
    export IMG_API
    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      if [[ -z "${debug}" ]]; then
        make docker-build-api
      else
        make docker-build-api-debug
      fi
    fi
    kind load docker-image --name "${cluster}" "${IMG_API}"

    if [[ -n "${use_local_registry}" ]]; then
      if [[ -z "${debug}" ]]; then
        make deploy-api-kind-local
      else
        make deploy-api-kind-local-debug
      fi
    else
      make deploy-api-kind
    fi

    create_tls_secret "korifi-api-ingress-cert" "korifi-api-system" "localhost"
  }
  popd >/dev/null
}

function deploy_kpack_image_builder() {
  if [[ -n "${api_only}" ]]; then return 0; fi

  pushd "${ROOT_DIR}/kpack-image-builder" >/dev/null
  {
    export KUBEBUILDER_ASSETS="${ROOT_DIR}/testbin/bin"
    echo "${PWD}"
    make generate
    IMG_KIB=${IMG_KIB:-"korifi-kpack-image-builder:$(uuidgen)"}
    export IMG_KIB
    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      make docker-build
    fi
    kind load docker-image --name "${cluster}" "${IMG_KIB}"

    if [[ -n "${use_local_registry}" ]]; then
      make deploy-on-kind
    else
      make deploy
    fi
  }
  popd >/dev/null

  kubectl rollout status deployment/korifi-kpack-build-controller-manager -w -n korifi-kpack-build-system
}

function deploy_statefulset_runner() {
  if [[ -n "${api_only}" ]]; then return 0; fi

  pushd "${ROOT_DIR}/statefulset-runner" >/dev/null
  {
    export KUBEBUILDER_ASSETS="${ROOT_DIR}/testbin/bin"
    echo "${PWD}"
    make generate
    IMG_SSR=${IMG_SSR:-"korifi-statefulset-runner:$(uuidgen)"}
    export IMG_SSR
    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      make docker-build
    fi
    kind load docker-image --name "${cluster}" "${IMG_SSR}"
    make deploy
  }
  popd >/dev/null

  kubectl rollout status deployment/korifi-statefulset-runner-controller-manager -w -n korifi-statefulset-runner-system
}

ensure_kind_cluster "${cluster}"
ensure_local_registry
install_dependencies

cat <<EOF
****************************************************
Building and deploying Korifi components in parallel

Logs will be shown when complete
****************************************************
EOF

tmp="$(mktemp -d)"
trap "rm -rf ${tmp}; clean_up_img_refs" EXIT
deploy_korifi_controllers &>"${tmp}/controllers" &
deploy_korifi_api &>"${tmp}/api" &
deploy_kpack_image_builder &>"${tmp}/kip" &
deploy_statefulset_runner &>"${tmp}/stsr" &
wait

cat <<EOF
***********
Controllers
***********
EOF
cat "${tmp}/controllers"

cat <<EOF
***********
API
***********
EOF
cat "${tmp}/api"

cat <<EOF
***********
Kpack Image Builder
***********
EOF
cat "${tmp}/kip"

cat <<EOF
***********
Stateful Set Runner
***********
EOF
cat "${tmp}/stsr"
