#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TEST_DIR="$SCRIPT_DIR/../tests"
DEP_DIR="$TEST_DIR/dependencies"
VENDOR_DIR="$TEST_DIR/vendor"

TEMP_FILES=()
trap 'for file in ${TEMP_FILES[@]-}; do rm -rf $file; done' EXIT

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

retry() {
  until $@; do
    echo -n .
    sleep 1
  done
  echo
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

retry kubectl apply -f "$VENDOR_DIR/kpack"
# Increase the CPU limit on the kpack-controller. Without this change the ClusterBuilder takes 10+ minutes to
# become ready on M1 Macs. With this change the ClusterBuilder becomes ready in the time it takes this script to run.
kubectl patch -n kpack deployment kpack-controller -p \
  '{"spec": {"template": {"spec": {"containers": [{"name": "controller", "resources": {"limits": {"cpu": "500m"}}}]}}}}'

echo "********************"
echo " Installing Contour"
echo "********************"

kubectl apply -f "$VENDOR_DIR/contour/contour-gateway-provisioner.yaml"

kubectl apply -f - <<EOF
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour
spec:
  controllerName: projectcontour.io/gateway-controller
EOF

if ! kubectl get apiservice v1beta1.metrics.k8s.io >/dev/null 2>&1; then
  if [[ "${INSECURE_TLS_METRICS_SERVER:-}" == "true" ]]; then
    echo "************************************************"
    echo " Installing Metrics Server Insecure TLS options"
    echo "************************************************"

    TEMP_FILES+=("$DEP_DIR/insecure-metrics-server/components.yaml")
    cp "$VENDOR_DIR/metrics-server-local/components.yaml" "$DEP_DIR/insecure-metrics-server/components.yaml"
    kubectl apply -k "$DEP_DIR/insecure-metrics-server"
  else
    echo "***************************"
    echo " Installing Metrics Server"
    echo "***************************"

    kubectl apply -f "$VENDOR_DIR/metrics-server-local"
  fi
fi
