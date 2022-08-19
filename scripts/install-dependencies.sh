#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEP_DIR="$(cd "${SCRIPT_DIR}/../tests/dependencies" && pwd)"

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
echo "Creating CF Namespace and cf-admin RoleBinding"
echo "************************************************"

kubectl apply -f "${DEP_DIR}/cf-setup.yaml"

echo "**************************"
echo "Creating user 'cf-admin'"
echo "**************************"

"$SCRIPT_DIR/create-new-user.sh" cf-admin

cert_manager_version=$(curl --silent "https://api.github.com/repos/cert-manager/cert-manager/releases/latest" | jq -r '.tag_name')
echo "*************************"
echo "Installing Cert Manager ${cert_manager_version}"
echo "*************************"

kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/${cert_manager_version}/cert-manager.yaml

kubectl -n cert-manager rollout status deployment/cert-manager --watch=true
kubectl -n cert-manager rollout status deployment/cert-manager-webhook --watch=true
kubectl -n cert-manager rollout status deployment/cert-manager-cainjector --watch=true

kpack_version=$(curl --silent "https://api.github.com/repos/pivotal/kpack/releases/latest" | jq -r '.tag_name' | tr -d 'v')
echo "*******************"
echo "Installing Kpack v${kpack_version}"
echo "*******************"

kubectl apply -f "https://github.com/pivotal/kpack/releases/download/v${kpack_version}/release-${kpack_version}.yaml"

echo "*******************"
echo "Configuring Kpack"
echo "*******************"

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

contour_version="$(curl -sL https://projectcontour.io/quickstart/contour.yaml | grep -Eo 'ghcr.io/projectcontour/contour:v[0-9.]+' | head -1 | cut -d: -f2)"
echo "*******************"
echo "Installing Contour ${contour_version}"
echo "*******************"

kubectl apply -f https://projectcontour.io/quickstart/contour.yaml

sbr_version=$(curl --silent "https://api.github.com/repos/servicebinding/runtime/releases/latest" | jq -r '.tag_name')
echo "**************************************"
echo "Installing Service Binding Runtime ${sbr_version}"
echo "**************************************"

kubectl apply -f https://github.com/servicebinding/runtime/releases/download/${sbr_version}/servicebinding-runtime-${sbr_version}.yaml
kubectl -n servicebinding-system rollout status deployment/servicebinding-controller-manager --watch=true
kubectl apply -f https://github.com/servicebinding/runtime/releases/download/${sbr_version}/servicebinding-workloadresourcemappings-${sbr_version}.yaml

if ! kubectl get apiservice v1beta1.metrics.k8s.io >/dev/null 2>&1; then
  if [[ -v INSECURE_TLS_METRICS_SERVER ]]; then
    echo "**************************************"
    echo "Installing Metrics Server v0.6.1 with Insecure TLS options"
    echo "**************************************"

    kubectl apply -f $DEP_DIR/metrics-server-local-0.6.1.yaml
  else
    metrics_server_version=$(curl --silent "https://api.github.com/repos/kubernetes-sigs/metrics-server/releases" | jq -r '.[].tag_name' | grep '^v' | head -1)
    echo "**************************************"
    echo "Installing Metrics Server ${metrics_server_version}"
    echo "**************************************"

    kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/download/${metrics_server_version}/components.yaml
  fi
fi
