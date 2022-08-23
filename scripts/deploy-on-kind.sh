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

  -s, --serial
      Deploy serially.
EOF
  exit 1
}

cluster=""
use_local_registry=""
controllers_only=""
api_only=""
default_domain=""
debug=""
serial=""

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
    -s | --serial)
      serial="true"
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

  cd "${ROOT_DIR}/job-task-runner"
  unset IMG_JTR
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
  # the htpasswd value below is username: user, password: password encoded using `htpasswd` binary
  # e.g. `docker run --entrypoint htpasswd httpd:2 -Bbn user password`
  helm upgrade --install localregistry twuni/docker-registry \
    --set service.type=NodePort,service.nodePort=30050,service.port=30050 \
    --set persistence.enabled=true \
    --set secrets.htpasswd='user:$2y$05$Ue5dboOfmqk6Say31Sin9uVbHWTl8J1Sgq9QyAEmFQRnq1TPfP1n2'
}

function install_dependencies() {
  if [[ -n "${controllers_only}" ]]; then return 0; fi
  if [[ -n "${api_only}" ]]; then return 0; fi

  pushd "${ROOT_DIR}" >/dev/null
  {
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

create_registry_secret() {
  echo "*********************************************"
  echo "Creating private registry secret"
  echo "*********************************************"

  if [[ -n "${use_local_registry}" ]]; then
    export DOCKER_SERVER="localregistry-docker-registry.default.svc.cluster.local:30050"
    export DOCKER_USERNAME="user"
    export DOCKER_PASSWORD="password"
  fi

  if [[ -n "${GCP_SERVICE_ACCOUNT_JSON_FILE:=}" ]]; then
    DOCKER_SERVER="gcr.io"
    DOCKER_USERNAME="_json_key"
    DOCKER_PASSWORD="$(cat ${GCP_SERVICE_ACCOUNT_JSON_FILE})"
  fi

  if [[ -n "${DOCKER_SERVER:=}" && -n "${DOCKER_USERNAME:=}" && -n "${DOCKER_PASSWORD:=}" ]]; then
    if kubectl get -n cf secret image-registry-credentials >/dev/null 2>&1; then
      kubectl delete -n cf secret image-registry-credentials
    fi

    kubectl create secret -n cf docker-registry image-registry-credentials \
      --docker-server=${DOCKER_SERVER} \
      --docker-username=${DOCKER_USERNAME} \
      --docker-password="${DOCKER_PASSWORD}"
  fi
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
        make deploy-controllers
      else
        make deploy-controllers
      fi
    else
      make deploy-controllers
    fi

    create_tls_secret "korifi-workloads-ingress-cert" "korifi-controllers-system" "*.vcap.me"
  }
  popd >/dev/null

  kubectl rollout status deployment/korifi-controllers-controller-manager -w -n korifi-controllers-system
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
        make deploy-api
      else
        make deploy-api
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

    make deploy
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

function deploy_job_task_runner() {
  if [[ -n "${api_only}" ]]; then return 0; fi

  pushd "${ROOT_DIR}/job-task-runner" >/dev/null
  {
    export KUBEBUILDER_ASSETS="${ROOT_DIR}/testbin/bin"
    echo "${PWD}"
    make generate
    IMG_JTR=${IMG_JTR:-"korifi-job-task-runner:$(uuidgen)"}
    export IMG_JTR
    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      make docker-build
    fi
    kind load docker-image --name "${cluster}" "${IMG_JTR}"
    make deploy
  }
  popd >/dev/null

  kubectl rollout status deployment/korifi-job-task-runner-controller-manager -w -n korifi-job-task-runner-system
}

ensure_kind_cluster "${cluster}"
ensure_local_registry
install_dependencies
deploy_korifi_controllers
make deploy-workloads
make deploy-kpack-config
create_registry_secret

if [[ -n "${serial}" ]]; then
  trap 'clean_up_img_refs' EXIT
  deploy_job_task_runner
  deploy_kpack_image_builder
  deploy_statefulset_runner
  deploy_korifi_api
else
  cat <<EOF
****************************************************
Building and deploying Korifi components in parallel

Logs will be shown when complete
****************************************************
EOF

  tmp="$(mktemp -d)"
  trap "rm -rf ${tmp}; clean_up_img_refs" EXIT
  deploy_job_task_runner &>"${tmp}/jtr" &
  deploy_kpack_image_builder &>"${tmp}/kip" &
  deploy_statefulset_runner &>"${tmp}/stsr" &
  deploy_korifi_api &>"${tmp}/api" &
  wait

  cat <<EOF
***********
Job Task Runner
***********
EOF
  cat "${tmp}/jtr"

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

  cat <<EOF
***********
API
***********
EOF
  cat "${tmp}/api"
fi
