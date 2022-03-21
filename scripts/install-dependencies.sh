#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEP_DIR="$(cd "${SCRIPT_DIR}/../dependencies" && pwd)"

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

echo "*********************************************"
echo "Creating CF Namespace and admin RoleBinding"
echo "*********************************************"

kubectl apply -f "${DEP_DIR}/cf-setup.yaml"

echo "***********************"
echo "Creating user 'admin'"
echo "***********************"

"$SCRIPT_DIR/create-new-user.sh" admin

echo "*************************"
echo "Installing Cert Manager"
echo "*************************"

# Install Cert Manager
kubectl apply -f "${DEP_DIR}/cert-manager.yaml"

echo "*******************"
echo "Installing Kpack"
echo "*******************"

kubectl apply -f "${DEP_DIR}/kpack-release-0.5.0.yaml"

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

if [[ -n "${DOCKER_SERVER:=}" ]]; then
  cat dependencies/kpack/cluster_builder.yaml | sed "s/tag: gcr.io/tag: ${DOCKER_SERVER}/g" >/tmp/clusterbuilder.yml
  kubectl apply -f /tmp/clusterbuilder.yml
else
  kubectl apply -f "${DEP_DIR}/kpack/cluster_builder.yaml"
fi


echo "***********************"
echo "Installing Cartographer"
echo "***********************"

kubectl apply -f "${DEP_DIR}/cartographer.0.2.0.yaml"

echo "*******************"
echo "Installing Contour"
echo "*******************"

kubectl apply -f "${DEP_DIR}/contour-1.19.1.yaml"

echo "*******************"
echo "Installing HNC"
echo "*******************"

readonly HNC_VERSION="v0.9.0"
readonly HNC_PLATFORM="$(go env GOHOSTOS)_$(go env GOHOSTARCH)"
readonly HNC_BIN="${PWD}/bin"
export PATH="${HNC_BIN}:${PATH}"

mkdir -p "${HNC_BIN}"
curl -L "https://github.com/kubernetes-sigs/hierarchical-namespaces/releases/download/${HNC_VERSION}/kubectl-hns_${HNC_PLATFORM}" -o "${HNC_BIN}/kubectl-hns"
chmod +x "${HNC_BIN}/kubectl-hns"

kubectl apply -f "https://github.com/kubernetes-sigs/hierarchical-namespaces/releases/download/${HNC_VERSION}/hnc-manager.yaml"
kubectl rollout status deployment/hnc-controller-manager -w -n hnc-system

retry() {
  until $@; do
    echo -n .
    sleep 1
  done
  echo
}

# Hierarchical namespace controller is quite asynchronous. There is no
# guarantee that the operations below would succeed on first invocation,
# so retry until they do.
echo -n waiting for hns controller to be ready and servicing validating webhooks
retry kubectl create namespace ping-hnc
retry kubectl hns create -n ping-hnc ping-hnc-child
retry kubectl get namespace ping-hnc-child
retry kubectl hns set --allowCascadingDeletion ping-hnc
retry kubectl delete namespace ping-hnc --wait=false

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

# Install image built based on eirini-controller/main@309b9c21 w/ values-template as default values file
helm template eirini-controller "${EIRINI_DIR}/deployment/helm" \
  --values "${EIRINI_DIR}/deployment/helm/values-template.yaml" \
  --set "webhooks.ca_bundle=${webhooks_ca_bundle}" \
  --set "workloads.default_namespace=cf" \
  --set "controller.registry_secret_name=image-registry-credentials" \
  --set "images.eirini_controller=eirini/eirini-controller@sha256:42e22b3222e9b3788782f5c141d260a5e163da4f4032e2926752ef2e5bae0685" \
  --namespace "eirini-controller" | kubectl apply -f -

echo "************************"
echo "Configuring Cartographer"
echo "************************"

kubectl apply -f "${DEP_DIR}/cartographer"

echo "******************************"
echo "Configuring Convention Service"
echo "******************************"

kubectl create ns cartographer-conventions-system
# Manually edit your /etc/hosts to map localregistry-docker-registry.default.svc.cluster.local to 127.0.0.1
export KO_DOCKER_REPO="localregistry-docker-registry.default.svc.cluster.local:30050/conventions"
# Check out the cartographer-conventions repo in your ~/workspace directory
kapp deploy -y -n cartographer-conventions-system -a controller -f <(cd ~/workspace/cartographer-conventions/; ytt -f dist/cartogrpaher-conventions.yaml -f dist/ca-overlay.yaml --data-value-file ca_cert_data=${CA_DATA:-dist/ca.pem} | ko resolve -f -)

echo "******"
echo "Done"
echo "******"
