#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TEST_DIR="$SCRIPT_DIR/../tests"
DEP_DIR="$TEST_DIR/dependencies"
VENDOR_DIR="$TEST_DIR/vendor"

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

kubectl apply -f "$VENDOR_DIR/gateway-api"
kubectl apply -f "$VENDOR_DIR/contour"
kubectl apply -f - <<EOF
kind: ConfigMap
apiVersion: v1
metadata:
  name: contour
  namespace: projectcontour
data:
  contour.yaml: |
    gateway:
      controllerName: projectcontour.io/gateway-controller
---
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1beta1
metadata:
  name: contour
spec:
  controllerName: projectcontour.io/gateway-controller
EOF
kubectl -n projectcontour rollout restart deployment/contour

# echo "********************"
# echo " Installing Istio   "
# echo "********************"
# kubectl apply -f "$VENDOR_DIR/gateway-api"
# istioctl install -y
# kubectl patch deployments.apps -n istio-system istio-ingressgateway -p '{"spec":{"template":{"spec":{"containers":[{"name":"istio-proxy","ports":[{"containerPort":8080,"hostPort":80},{"containerPort":8443,"hostPort":443}]}]}}}}'

echo "************************************"
echo " Installing Service Binding Runtime"
echo "************************************"

kubectl apply -f "$VENDOR_DIR/service-binding/servicebinding-runtime-v"*".yaml"
kubectl -n servicebinding-system rollout status deployment/servicebinding-controller-manager --watch=true
kubectl apply -f "$VENDOR_DIR/service-binding/servicebinding-workloadresourcemappings-v"*".yaml"

if ! kubectl get apiservice v1beta1.metrics.k8s.io >/dev/null 2>&1; then
  if [[ "${INSECURE_TLS_METRICS_SERVER:-}" == "true" ]]; then
    echo "************************************************"
    echo " Installing Metrics Server Insecure TLS options"
    echo "************************************************"

    trap "rm $DEP_DIR/insecure-metrics-server/components.yaml" EXIT
    cp "$VENDOR_DIR/metrics-server-local/components.yaml" "$DEP_DIR/insecure-metrics-server"
    kubectl apply -k "$DEP_DIR/insecure-metrics-server"
  else
    echo "***************************"
    echo " Installing Metrics Server"
    echo "***************************"

    kubectl apply -f "$VENDOR_DIR/metrics-server-local"
  fi
fi
