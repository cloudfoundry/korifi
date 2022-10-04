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
      wires up ports for remote debugging:
        localhost:30051 (controllers)
        localhost:30052 (api)
        localhost:30053 (kpack-image-builder)
        localhost:30054 (statefulset-runner)
        localhost:30055 (job-task-runner)

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
  echo
  echo "Resetting image references..."
  cd "${ROOT_DIR}"
  unset IMG_CONTROLLERS
  unset IMG_API
  unset IMG_JTR
  unset IMG_KIB
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
  - containerPort: 30053
    hostPort: 30053
    protocol: TCP
  - containerPort: 30054
    hostPort: 30054
    protocol: TCP
  - containerPort: 30055
    hostPort: 30055
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
    if [[ -n "${use_local_registry}" ]]; then
      export DOCKER_SERVER="localregistry-docker-registry.default.svc.cluster.local:30050"
      export DOCKER_USERNAME="user"
      export DOCKER_PASSWORD="password"
      export KPACK_TAG="localregistry-docker-registry.default.svc.cluster.local:30050/cf-relint-greengrass/korifi/kpack/beta"
    fi

    export INSECURE_TLS_METRICS_SERVER=true

    "${SCRIPT_DIR}/install-dependencies.sh"
  }
  popd >/dev/null
}

function deploy_korifi_controllers() {
  if [[ -n "${api_only}" ]]; then return 0; fi
  echo "Deploying korifi-controllers..."

  pushd "${ROOT_DIR}" >/dev/null
  {
    export IMG_CONTROLLERS=${IMG_CONTROLLERS:-"korifi-controllers:$(korifiCodeSha)"}
    export KUBEBUILDER_ASSETS="${ROOT_DIR}/testbin/bin"

    make generate-controllers

    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      if [[ -z "${debug}" ]]; then
        make docker-build-controllers
      else
        make docker-build-controllers-debug
      fi
    fi

    CLUSTER_NAME="$cluster" make kind-load-controllers-image

    if [[ -n "${use_local_registry}" ]]; then
      if [[ -z "${debug}" ]]; then
        make deploy-controllers-kind-local
      else
        make deploy-controllers-kind-local-debug
      fi
    else
      make deploy-controllers
    fi

    create_tls_cert "korifi-workloads-ingress-cert" "korifi-controllers" "\*.vcap.me"
  }
  popd >/dev/null

  kubectl rollout status deployment/korifi-controllers-controller-manager -w -n korifi-controllers-system

  if [[ -n "${default_domain}" ]]; then
    sed 's/vcap\.me/'${APP_FQDN:-vcap.me}'/' ${CONTROLLER_DIR}/config/samples/cfdomain.yaml | kubectl apply -f-
  fi
}

function korifiCodeSha() {
  find "$ROOT_DIR" -type f -name "*.go" -print0 | sort -z | xargs -0 sha1sum | sha1sum | cut -d " " -f 1

}

function deploy_korifi_api() {
  if [[ -n "${controllers_only}" ]]; then return 0; fi
  echo "Deploying korifi-api..."

  pushd "${ROOT_DIR}" >/dev/null
  {
    export IMG_API=${IMG_API:-"korifi-api:$(korifiCodeSha)"}

    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      if [[ -z "${debug}" ]]; then
        make docker-build-api
      else
        make docker-build-api-debug
      fi
    fi

    CLUSTER_NAME="$cluster" make kind-load-api-image

    if [[ -n "${use_local_registry}" ]]; then
      if [[ -z "${debug}" ]]; then
        make deploy-api-kind-local
      else
        make deploy-api-kind-local-debug
      fi
    else
      make deploy-api-kind
    fi

    create_tls_cert "korifi-api-ingress-cert" "korifi-api" "api.vcap.me"
  }
  popd >/dev/null
}

function deploy_job_task_runner() {
  if [[ -n "${api_only}" ]]; then return 0; fi
  echo "Deploying job-task-runner..."

  pushd "${ROOT_DIR}/job-task-runner" >/dev/null
  {
    export IMG_JTR=${IMG_JTR:-"korifi-job-task-runner:$(korifiCodeSha)"}
    export KUBEBUILDER_ASSETS="${ROOT_DIR}/testbin/bin"

    make generate

    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      if [[ -z "${debug}" ]]; then
        make docker-build
      else
        make docker-build-debug
      fi
    fi

    CLUSTER_NAME="$cluster" make kind-load-image

    if [[ -n "${debug}" ]]; then
      make deploy-kind-local-debug
    else
      make deploy
    fi
  }
  popd >/dev/null

  kubectl rollout status deployment/korifi-job-task-runner-controller-manager -w -n korifi-job-task-runner-system
}

function deploy_kpack_image_builder() {
  if [[ -n "${api_only}" ]]; then return 0; fi
  echo "Deploying kpack-image-builder..."

  pushd "${ROOT_DIR}/kpack-image-builder" >/dev/null
  {
    export IMG_KIB=${IMG_KIB:-"korifi-kpack-image-builder:$(korifiCodeSha)"}
    export KUBEBUILDER_ASSETS="${ROOT_DIR}/testbin/bin"

    make generate

    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      if [[ -z "${debug}" ]]; then
        make docker-build
      else
        make docker-build-debug
      fi
    fi

    CLUSTER_NAME="$cluster" make kind-load-image

    if [[ -n "${use_local_registry}" ]]; then
      if [[ -z "${debug}" ]]; then
        make deploy-kind-local
      else
        make deploy-kind-local-debug
      fi
    else
      make deploy
    fi
  }
  popd >/dev/null
}

function deploy_statefulset_runner() {
  if [[ -n "${api_only}" ]]; then return 0; fi
  echo "Deploying statefulset-runner..."

  pushd "${ROOT_DIR}/statefulset-runner" >/dev/null
  {
    export IMG_SSR=${IMG_SSR:-"korifi-statefulset-runner:$(korifiCodeSha)"}
    export KUBEBUILDER_ASSETS="${ROOT_DIR}/testbin/bin"

    make generate

    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      if [[ -z "${debug}" ]]; then
        make docker-build
      else
        make docker-build-debug
      fi
    fi

    CLUSTER_NAME="$cluster" make kind-load-image

    if [[ -n "${debug}" ]]; then
      make deploy-kind-local-debug
    else
      make deploy
    fi
  }
  popd >/dev/null
}

ensure_kind_cluster "${cluster}"
ensure_local_registry
install_dependencies

trap 'clean_up_img_refs' EXIT
deploy_korifi_controllers
deploy_job_task_runner
deploy_kpack_image_builder
deploy_statefulset_runner
deploy_korifi_api
