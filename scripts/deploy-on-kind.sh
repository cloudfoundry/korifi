#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT_DIR="${ROOT_DIR}/scripts"

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
      - PACKAGE_REPOSITORY
      - DROPLET_REPOSITORY
      - KPACK_BUILDER_REPOSITORY

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

  -s, --use-registry-service-account
      Use a service account credentials to access the registry (testing not using secrets)

EOF
  exit 1
}

cluster=""
use_custom_registry=""
debug=""
use_registry_service_account=""

while [[ $# -gt 0 ]]; do
  i=$1
  case $i in
    -r | --use-custom-registry)
      use_custom_registry="true"
      # blow up if required vars not set
      echo "$DOCKER_SERVER $DOCKER_USERNAME $DOCKER_PASSWORD $PACKAGE_REPOSITORY $DROPLET_REPOSITORY $KPACK_BUILDER_REPOSITORY" >/dev/null
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
    -s | --use-registry-service-account)
      use_registry_service_account="true"
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
  if [[ -n "${use_custom_registry}" ]]; then return 0; fi

  local sethtpasswd="--set secrets.htpasswd='user:\$2y\$05\$Ue5dboOfmqk6Say31Sin9uVbHWTl8J1Sgq9QyAEmFQRnq1TPfP1n2'"
  if [[ -n "${use_registry_service_account}" ]]; then
    sethtpasswd=""
  fi

  helm repo add twuni https://helm.twun.io
  # the htpasswd value below is username: user, password: password encoded using `htpasswd` binary
  # e.g. `docker run --entrypoint htpasswd httpd:2 -Bbn user password`
  helm upgrade --install localregistry twuni/docker-registry \
    --set service.type=NodePort,service.nodePort=30050,service.port=30050 \
    --set persistence.enabled=true \
    $sethtpasswd

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
    if [[ -n "${debug}" ]]; then
      doDebug="true"
    fi

    if [[ -n "$use_custom_registry" ]]; then
      helm upgrade --install korifi helm/korifi \
        --namespace korifi \
        --values=scripts/assets/values.yaml \
        --set=global.debug="$doDebug" \
        --set=api.packageRepository="$PACKAGE_REPOSITORY" \
        --set=kpack-image-builder.dropletRepository="$DROPLET_REPOSITORY" \
        --set=kpack-image-builder.builderRepository="$KPACK_BUILDER_REPOSITORY" \
        --wait
    else
      registry_configuration=""
      if [[ -n "${use_registry_service_account}" ]]; then
        registry_configuration=(
          '--set' 'global.containerRegistrySecret='
          '--set' 'global.containerRegistryServiceAccount="registry-service-account"'
        )
      fi

      helm upgrade --install korifi helm/korifi \
        --namespace korifi \
        --values=scripts/assets/values.yaml \
        --set=global.debug="$doDebug" \
        "${registry_configuration[@]}" \
        --wait
    fi
  }
  popd >/dev/null
}

function create_namespaces() {
  for ns in cf korifi; do
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  labels:
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/enforce: restricted
  name: $ns
EOF
  done

  if [[ -z "${use_custom_registry}" ]]; then
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

function create_registry_service_account() {
  cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  annotations:
    cloudfoundry.org/propagate-service-account: "true"
  name: registry-service-account
  namespace: cf
EOF

  cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ServiceAccount
metadata:
  name: registry-service-account
  namespace: korifi
EOF
}

ensure_kind_cluster "${cluster}"
ensure_local_registry
install_dependencies
create_namespaces
if [[ -n "${use_registry_service_account}" ]]; then
  create_registry_service_account
fi
deploy_korifi
