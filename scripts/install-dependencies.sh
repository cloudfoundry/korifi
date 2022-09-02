#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TEST_DIR="$SCRIPT_DIR/../tests"
DEP_DIR="$TEST_DIR/dependencies"
VENDOR_DIR="$TEST_DIR/vendor"

source "$SCRIPT_DIR/common.sh"

function usage_text() {
  cat <<EOF
Usage:
  $(basename "$0")

flags:
  -g, --gcr-service-account-json
      (optional) Filepath to the GCP Service Account JSON describing a service account
      that has permissions to write to the project's container repository.
  -i, --insecure-tls-metrics-server
      (optional) Provide insecure TLS args to Metrics Server. This is useful for distributions such as Kind, Minikube, etc.
EOF
  exit 1
}

while [[ $# -gt 0 ]]; do
  i=$1
  case $i in
    -g=* | --gcr-service-account-json=*)
      GCP_SERVICE_ACCOUNT_JSON_FILE="${i#*=}"
      shift
      ;;
    -g | --gcr-service-account-json)
      GCP_SERVICE_ACCOUNT_JSON_FILE="${2}"
      shift 2
      ;;
    -i | --insecure-tls-metrics-server)
      INSECURE_TLS_METRICS_SERVER=true
      shift
      ;;
    *)
      echo -e "Error: Unknown flag: ${i/=*/}\n" >&2
      usage_text >&2
      exit 1
      ;;
  esac
done

echo "************************************************"
echo " Creating CF Namespace and cf-admin RoleBinding"
echo "************************************************"

kubectl apply -f "${DEP_DIR}/cf-setup.yaml"

echo "**************************"
echo " Creating 'cf-admin' user"
echo "**************************"

"$SCRIPT_DIR/create-new-user.sh" cf-admin

echo "*************************"
echo " Installing Cert Manager"
echo "*************************"

kubectl apply -f "$VENDOR_DIR/cert-manager"

kubectl -n cert-manager rollout status deployment/cert-manager --watch=true
kubectl -n cert-manager rollout status deployment/cert-manager-webhook --watch=true
kubectl -n cert-manager rollout status deployment/cert-manager-cainjector --watch=true

echo "******************"
echo " Installing Kpack"
echo "******************"

kubectl apply -f "$VENDOR_DIR/kpack"
# Increase the CPU limit on the kpack-controller. Without this change the ClusterBuilder takes 10+ minutes to
# become ready on M1 Macs. With this change the ClusterBuilder becomes ready in the time it takes this script to run.
kubectl patch -n kpack deployment kpack-controller -p \
  '{"spec": {"template": {"spec": {"containers": [{"name": "controller", "resources": {"limits": {"cpu": "500m"}}}]}}}}'

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

kubectl -n kpack wait --for condition=established --timeout=60s crd/clusterbuilders.kpack.io
kubectl -n kpack wait --for condition=established --timeout=60s crd/clusterstores.kpack.io
kubectl -n kpack wait --for condition=established --timeout=60s crd/clusterstacks.kpack.io

kubectl apply -f "${DEP_DIR}/kpack/service_account.yaml"
kubectl apply -f "${DEP_DIR}/kpack/cluster_stack.yaml" \
  -f "${DEP_DIR}/kpack/cluster_store.yaml"

if [[ -n "${KPACK_TAG:=}" ]]; then
  sed "s|tag: gcr\.io.*$|tag: $KPACK_TAG|" "$DEP_DIR/kpack/cluster_builder.yaml" | kubectl apply -f-
else
  kubectl apply -f "${DEP_DIR}/kpack/cluster_builder.yaml"
fi

echo "********************"
echo " Installing Contour"
echo "********************"

# Temporarily resolve an issue with contour running on Apple silicon.
# This fix can be removed once the latest version of contour uses envoy v1.23.1 or newer
if command -v kbld &>/dev/null; then
  kbld --image-map-file "${DEP_DIR}/contour/kbld-image-mapping-to-fix-envoy-v1.23-bug.json" -f "$VENDOR_DIR/contour" | kubectl apply -f -
else
  kubectl apply -f "$VENDOR_DIR/contour"
fi

echo "************************************"
echo " Installing Service Binding Runtime"
echo "************************************"

kubectl apply -f "$VENDOR_DIR/service-binding/servicebinding-runtime-v*.yaml"
kubectl -n servicebinding-system rollout status deployment/servicebinding-controller-manager --watch=true
kubectl apply -f "$VENDOR_DIR/service-binding/servicebinding-workloadresourcemappings-v*.yaml"

if ! kubectl get apiservice v1beta1.metrics.k8s.io >/dev/null 2>&1; then
  if [[ -v INSECURE_TLS_METRICS_SERVER ]]; then
    echo "************************************************"
    echo " Installing Metrics Server Insecure TLS options"
    echo "************************************************"

    trap "rm $DEP_DIR/insecure-metrics-server/components.yaml" EXIT
    cp "$VENDOR_DIR/metrics-server-local/components.yaml" "$DEP_DIR/insecure-metrics-server/components.yaml"
    kubectl apply -k "$DEP_DIR/insecure-metrics-server"
  else
    echo "***************************"
    echo " Installing Metrics Server"
    echo "***************************"

    kubectl apply -f "$VENDOR_DIR/metrics-server-local"
  fi
fi
