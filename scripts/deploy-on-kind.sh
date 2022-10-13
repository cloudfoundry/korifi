#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT_DIR="${ROOT_DIR}/scripts"

function usage_text() {
  cat <<EOF
Usage:
  $(basename "$0") <kind cluster name>

flags:
  -l, --use-local-registry
      Deploys a local container registry to the kind cluster.

  -v, --verbose
      Verbose output (bash -x).

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
debug=""

while [[ $# -gt 0 ]]; do
  i=$1
  case $i in
    -l | --use-local-registry)
      use_local_registry="true"
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

function ensure_kind_cluster() {
  if ! kind get clusters | grep -q "${cluster}"; then
    kind create cluster --name "${cluster}" --wait 5m --config="$SCRIPT_DIR/assets/kind-config.yaml"
  fi

  kind export kubeconfig --name "${cluster}"
}

function ensure_local_registry() {
  if [[ -z "${use_local_registry}" ]]; then return 0; fi

  helm repo add twuni https://helm.twun.io
  # the htpasswd value below is username: user, password: password encoded using `htpasswd` binary
  # e.g. `docker run --entrypoint htpasswd httpd:2 -Bbn user password`
  helm upgrade --install localregistry twuni/docker-registry \
    --set service.type=NodePort,service.nodePort=30050,service.port=30050 \
    --set persistence.enabled=true \
    --set secrets.htpasswd='user:$2y$05$Ue5dboOfmqk6Say31Sin9uVbHWTl8J1Sgq9QyAEmFQRnq1TPfP1n2'
}

function install_dependencies() {
  pushd "${ROOT_DIR}" >/dev/null
  {
    export INSECURE_TLS_METRICS_SERVER=true

    "${SCRIPT_DIR}/install-dependencies.sh"
  }
  popd >/dev/null
}

function deploy_korifi() {
  pushd "${ROOT_DIR}" >/dev/null
  {

    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      echo "Building korifi values file..."

      make generate manifests

      kbld_file="scripts/assets/korifi-kbld.yml"
      if [[ -n "$debug" ]]; then
        kbld_file="scripts/assets/korifi-debug-kbld.yml"
      fi

      kbld \
        -f "$kbld_file" \
        -f "scripts/assets/values-template.yaml" \
        --images-annotation=false >"scripts/assets/values.yaml"

      awk '/image:/ {print $2}' scripts/assets/values.yaml | while read -r img; do
        kind load docker-image --name "$cluster" "$img"
      done
    fi

    echo "Deploying korifi..."
    helm dependency update helm/korifi

    doDebug="false"
    secLevel="restricted"
    if [[ -n "${debug}" ]]; then
      doDebug="true"
      secLevel="privileged"
    fi

    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  labels:
    pod-security.kubernetes.io/enforce: $secLevel
  name: korifi
EOF

    helm upgrade --install korifi helm/korifi \
      --namespace korifi \
      --values=scripts/assets/values.yaml \
      --set=global.debug="$doDebug" \
      --wait
  }
  popd >/dev/null
}

function create_registry_secret() {
  if [[ -n "${use_local_registry}" ]]; then
    DOCKER_SERVER="localregistry-docker-registry.default.svc.cluster.local:30050"
    DOCKER_USERNAME="user"
    DOCKER_PASSWORD="password"
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

ensure_kind_cluster "${cluster}"
ensure_local_registry
install_dependencies
deploy_korifi
create_registry_secret
