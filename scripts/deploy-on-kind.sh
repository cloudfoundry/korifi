#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT_DIR="${ROOT_DIR}/scripts"
API_DIR="${ROOT_DIR}/api"
CONTROLLER_DIR="${ROOT_DIR}/controllers"
export PATH="${PATH}:${API_DIR}/bin"

OPENSSL_VERSION="$(openssl version | awk '{ print $1 }')"

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
      Builds controller image with debugging hooks and
      wires up localhost:30051 for remote debugging.
EOF
  exit 1
}

cluster=""
use_local_registry=""
controllers_only=""
api_only=""
default_domain=""
controllers_debug=""
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
      controllers_debug="true"
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

if [[ -n "${controllers_debug}" ]]; then
  if [[ -z "${use_local_registry}" ]]; then
    echo -e "Error: currently controller debugging requires local registry (only because Kustomize is hard, not for real reasons)" >&2
    exit 1
  fi
fi


function create_tls_secret() {
  local secret_name=${1:?}
  local secret_namespace=${2:?}
  local tls_cn=${3:?}

  tmp_dir=$(mktemp -d -t cf-tls-XXXXXX)

  if [[ "${OPENSSL_VERSION}" == "OpenSSL" ]]; then
    openssl req -x509 -newkey rsa:4096 \
      -keyout ${tmp_dir}/tls.key \
      -out ${tmp_dir}/tls.crt \
      -nodes \
      -subj "/CN=${tls_cn}" \
      -addext "subjectAltName = DNS:${tls_cn}" \
      -days 365
  else
    openssl req -x509 -newkey rsa:4096 \
      -keyout ${tmp_dir}/tls.key \
      -out ${tmp_dir}/tls.crt \
      -nodes \
      -subj "/CN=${tls_cn}" \
      -extensions SAN -config <(cat /etc/ssl/openssl.cnf <(printf "[ SAN ]\nsubjectAltName='DNS:${tls_cn}'")) \
      -days 365
  fi

  cat <<EOF >${tmp_dir}/kustomization.yml
secretGenerator:
- name: ${secret_name}
  namespace: ${secret_namespace}
  files:
  - tls.crt=tls.crt
  - tls.key=tls.key
  type: "kubernetes.io/tls"
generatorOptions:
  disableNameSuffixHash: true
EOF

  kubectl apply -k $tmp_dir

  rm -r ${tmp_dir}
}

# undo *_IMG changes in config and reference
function clean_up_img_refs() {
  cd "${ROOT_DIR}"
  unset IMG_CONTROLLERS
  unset IMG_API
  make build-reference
}
trap clean_up_img_refs EXIT

function ensure_kind_cluster() {
  if [[ -n "${controllers_only}" ]]; then return 0; fi
  if [[ -n "${api_only}" ]]; then return 0; fi

  if ! kind get clusters | grep -q "${cluster}"; then
    cat <<EOF | kind create cluster --name "${cluster}" --wait 5m --config=-
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
  - containerPort: 30051
    hostPort: 30051
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
  if [[ -n "${controllers_only}" ]]; then return 0; fi
  if [[ -n "${api_only}" ]]; then return 0; fi

  pushd "${ROOT_DIR}" >/dev/null
  {
    if [[ -n "${use_local_registry}" ]]; then
      export DOCKER_SERVER="localregistry-docker-registry.default.svc.cluster.local:30050"
      export DOCKER_USERNAME="whatevs"
      export DOCKER_PASSWORD="whatevs"
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

function deploy_cf_k8s_controllers() {
  if [[ -n "${api_only}" ]]; then return 0; fi

  pushd "${ROOT_DIR}" >/dev/null
  {
    export KUBEBUILDER_ASSETS="${ROOT_DIR}/testbin/bin"
    echo "${PWD}"
    make generate-controllers
    IMG_CONTROLLERS=${IMG_CONTROLLERS:-"cf-k8s-controllers:$(uuidgen)"}
    export IMG_CONTROLLERS
    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      if [[ -z "${controllers_debug}" ]]; then
        make docker-build-controllers
      else
        make docker-build-controllers-debug
      fi
    fi
    kind load docker-image --name "${cluster}" "${IMG_CONTROLLERS}"

    make install-crds
    if [[ -n "${use_local_registry}" ]]; then
      if [[ -z "${controllers_debug}" ]]; then
        make deploy-controllers-kind-local
      else
        make deploy-controllers-kind-local-debug
      fi
    else
      make deploy-controllers
    fi

    create_tls_secret "cf-k8s-workloads-ingress-cert" "cf-k8s-controllers-system" "*.vcap.me"
  }
  popd >/dev/null

  if [[ -n "${default_domain}" ]]; then
    retry kubectl apply -f "${CONTROLLER_DIR}/config/samples/cfdomain.yaml"
  fi
}

function deploy_cf_k8s_api() {
  if [[ -n "${controllers_only}" ]]; then return 0; fi

  pushd "${ROOT_DIR}" >/dev/null
  {
    IMG_API=${IMG_API:-"cf-k8s-api:$(uuidgen)"}
    export IMG_API
    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      make docker-build-api
    fi
    kind load docker-image --name "${cluster}" "${IMG_API}"

    if [[ -n "${use_local_registry}" ]]; then
      make deploy-api-kind-local
    else
      make deploy-api-kind-auth
    fi

    create_tls_secret "cf-k8s-api-ingress-cert" "cf-k8s-api-system" "localhost"
  }
  popd >/dev/null
}

ensure_kind_cluster "${cluster}"
ensure_local_registry
install_dependencies
deploy_cf_k8s_controllers
deploy_cf_k8s_api

