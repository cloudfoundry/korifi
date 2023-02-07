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
  -i, --insecure-tls-metrics-server
      (optional) Provide insecure TLS args to Metrics Server. This is useful for distributions such as Kind, Minikube, etc.
EOF
  exit 1
}

while [[ $# -gt 0 ]]; do
  i=$1
  case $i in
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

echo "**************************"
echo " Creating 'cf-admin' user"
echo "**************************"

if [[ "${CLUSTER_TYPE:-}" != "EKS" ]]; then
  "$SCRIPT_DIR/create-new-user.sh" cf-admin
fi

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

kubectl apply -f "$VENDOR_DIR/service-binding/servicebinding-runtime-v"*".yaml"
kubectl -n servicebinding-system rollout status deployment/servicebinding-controller-manager --watch=true
kubectl apply -f "$VENDOR_DIR/service-binding/servicebinding-workloadresourcemappings-v"*".yaml"

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
