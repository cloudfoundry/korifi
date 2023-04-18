#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT_DIR="${ROOT_DIR}/scripts"

# workaround for https://github.com/carvel-dev/kbld/issues/213
# kbld fails with git error messages in languages than other english
export LC_ALL=en_US.UTF-8

function usage_text() {
  cat <<EOF
Usage:
  $(basename "$0") <kind cluster name>

flags:
  -r, --use-custom-registry
      Instead of using the default local registry, use the registry
      described by the follow set of env vars:
      - DOCKER_SERVER
      - DOCKER_USERNAME
      - DOCKER_PASSWORD
      - REPOSITORY_PREFIX
      - KPACK_BUILDER_REPOSITORY

  -v, --verbose
      Verbose output (bash -x).

  -D, --debug
      Builds controller and api images with debugging hooks and
      wires up ports for remote debugging:
        localhost:30051 (controllers)
        localhost:30052 (api)

  -s, --use-registry-service-account
      Use a service account credentials to access the registry (testing not using secrets)

EOF
  exit 1
}

cluster=""
use_custom_registry=""
debug="false"

while [[ $# -gt 0 ]]; do
  i=$1
  case $i in
    -r | --use-custom-registry)
      use_custom_registry="true"
      # blow up if required vars not set
      echo "$DOCKER_SERVER $DOCKER_USERNAME $DOCKER_PASSWORD $REPOSITORY_PREFIX $KPACK_BUILDER_REPOSITORY" >/dev/null
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

function ensure_kind_cluster() {
  if ! kind get clusters | grep -q "${cluster}"; then
    kind create cluster --name "${cluster}" --wait 5m --config="$SCRIPT_DIR/assets/kind-config.yaml"
  fi

  kind export kubeconfig --name "${cluster}"
}

function ensure_local_registry() {
  if [[ -n "${use_custom_registry}" ]]; then
    return 0
  fi

  helm repo add twuni https://helm.twun.io
  # the htpasswd value below is username: user, password: password encoded using `htpasswd` binary
  # e.g. `docker run --entrypoint htpasswd httpd:2 -Bbn user password`
  helm upgrade --install localregistry twuni/docker-registry \
    --set service.type=NodePort,service.nodePort=30050,service.port=30050 \
    --set persistence.enabled=true \
    --set persistence.deleteEnabled=true \
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
      if [[ "$debug" == "true" ]]; then
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

    REPOSITORY_PREFIX=${REPOSITORY_PREFIX:-"localregistry-docker-registry.default.svc.cluster.local:30050/"}
    KPACK_BUILDER_REPOSITORY=${KPACK_BUILDER_REPOSITORY:-"localregistry-docker-registry.default.svc.cluster.local:30050/kpack-builder"}

    helm upgrade --install korifi helm/korifi \
      --namespace korifi \
      --values=scripts/assets/values.yaml \
      --set=global.debug="$debug" \
      --set=global.containerRepositoryPrefix="$REPOSITORY_PREFIX" \
      --set=kpackImageBuilder.builderRepository="$KPACK_BUILDER_REPOSITORY" \
      --wait
  }
  popd >/dev/null
}

function create_namespaces() {
  local security_policy

  security_policy="restricted"
  for ns in cf korifi; do
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  labels:
    pod-security.kubernetes.io/audit: $security_policy
    pod-security.kubernetes.io/enforce: $security_policy
  name: $ns
EOF
  done
}

function create_registry_secret() {
  DOCKER_SERVER=${DOCKER_SERVER:-"localregistry-docker-registry.default.svc.cluster.local:30050"}
  DOCKER_USERNAME=${DOCKER_USERNAME:-"user"}
  DOCKER_PASSWORD=${DOCKER_PASSWORD:-"password"}

  kubectl delete -n cf secret image-registry-credentials --ignore-not-found
  kubectl create secret -n cf docker-registry image-registry-credentials \
    --docker-server="${DOCKER_SERVER}" \
    --docker-username="${DOCKER_USERNAME}" \
    --docker-password="${DOCKER_PASSWORD}"
}

ensure_kind_cluster "${cluster}"
ensure_local_registry
install_dependencies
create_namespaces
create_registry_secret
deploy_korifi
