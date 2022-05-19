#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEP_DIR="$(cd "${SCRIPT_DIR}/../dependencies" && pwd)"

source "$SCRIPT_DIR/common.sh"

function usage_text() {
  cat <<EOF
Usage:
  $(basename "$0")

flags:
  -g, --gcr-service-account-json
      (optional) Filepath to the GCP Service Account JSON describing a service account
      that has permissions to write to the project's container repository.

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
    shift
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

echo "*************************"
echo "Installing Cert Manager"
echo "*************************"

# Install Cert Manager
kubectl apply -f "${DEP_DIR}/cert-manager.yaml"

echo "*******************"
echo "Installing Kpack"
echo "*******************"

kubectl apply -f "${DEP_DIR}/kpack-release-0.5.2.yaml"

echo "*******************"
echo "Configuring Kpack"
echo "*******************"

if [[ -n "${GCP_SERVICE_ACCOUNT_JSON_FILE:=}" ]]; then
  DOCKER_SERVER="gcr.io"
  DOCKER_USERNAME="_json_key"
  DOCKER_PASSWORD="$(cat ${GCP_SERVICE_ACCOUNT_JSON_FILE})"
fi
if [[ -n "${DOCKER_SERVER:=}" && -n "${DOCKER_USERNAME:=}" && -n "${DOCKER_PASSWORD:=}" ]]; then
  for ns in cf; do
    if kubectl get -n $ns secret image-registry-credentials >/dev/null 2>&1; then
      kubectl delete -n $ns secret image-registry-credentials
    fi

    kubectl create secret -n $ns docker-registry image-registry-credentials \
      --docker-server=${DOCKER_SERVER} \
      --docker-username=${DOCKER_USERNAME} \
      --docker-password="${DOCKER_PASSWORD}"
  done
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

echo "*******************"
echo "Installing Contour"
echo "*******************"

kubectl apply -f "${DEP_DIR}/contour-1.19.1.yaml"

echo "*******************"
echo "Installing HNC"
echo "*******************"

kubectl apply -k ${DEP_DIR}/hnc/cf
kubectl rollout status deployment/hnc-controller-manager -w -n hnc-system

# install kubectl addon in order to configure secrets propagation
readonly HNC_VERSION="v1.0.0"
readonly HNC_PLATFORM="$(go env GOHOSTOS)_$(go env GOHOSTARCH)"
readonly HNC_BIN="${PWD}/bin"
export PATH="${HNC_BIN}:${PATH}"

mkdir -p "${HNC_BIN}"
curl -L "https://github.com/kubernetes-sigs/hierarchical-namespaces/releases/download/${HNC_VERSION}/kubectl-hns_${HNC_PLATFORM}" -o "${HNC_BIN}/kubectl-hns"
chmod +x "${HNC_BIN}/kubectl-hns"

# Propagate the kpack image registry write secret
retry kubectl hns config set-resource secrets --mode Propagate

echo "*******************"
echo "Installing Eirini"
echo "*******************"

## Assumes eirini-controller repository is available at the same level as this project's repository in the filesystem
## Make sure you have the latest copy of the repository
EIRINI_DIR="$(cd "$(dirname "$0")/../../eirini-controller" && pwd)"

"${SCRIPT_DIR}/generate-eirini-certs-secret.sh" "*.eirini-controller.svc"

webhooks_ca_bundle="$(kubectl get secret -n eirini-controller eirini-webhooks-certs -o jsonpath="{.data['tls\.ca']}")"

# Install image built based on eirini-controller/master@dd915fd80a13297cd727262ae7917d42a5d4375a w/ values-template as default values file
helm template eirini-controller "${EIRINI_DIR}/deployment/helm" \
  --values "${EIRINI_DIR}/deployment/helm/values-template.yaml" \
  --set "webhooks.ca_bundle=${webhooks_ca_bundle}" \
  --set "workloads.default_namespace=cf" \
  --set "controller.registry_secret_name=image-registry-credentials" \
  --set "images.eirini_controller=eirini/eirini-controller@sha256:7b2ad538d603589de2539c186439919ad004d901fc7067302769799bf4309d30" \
  --namespace "eirini-controller" | kubectl apply -f -

echo "**************************************"
echo "Installing Service Binding Controller"
echo "**************************************"

kubectl apply -f "${DEP_DIR}/service-bindings-0.7.1.yaml"

echo "******"
echo "Done"
echo "******"
