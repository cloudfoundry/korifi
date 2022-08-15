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

echo "**************************"
echo "Creating user 'cf-admin'"
echo "**************************"

"$SCRIPT_DIR/create-new-user.sh" cf-admin

echo "*************************"
echo "Installing Cert Manager"
echo "*************************"

# Install Cert Manager
kubectl apply -f "${DEP_DIR}/cert-manager.yaml"
kubectl -n cert-manager rollout status deployment/cert-manager --watch=true
kubectl -n cert-manager rollout status deployment/cert-manager-webhook --watch=true
kubectl -n cert-manager rollout status deployment/cert-manager-cainjector --watch=true

echo "*******************"
echo "Installing Kpack"
echo "*******************"

kubectl apply -f "${DEP_DIR}/kpack-release-0.6.0.yaml"

echo "*******************"
echo "Installing Contour"
echo "*******************"

kubectl apply -f "${DEP_DIR}/contour-1.19.1.yaml"

echo "**************************************"
echo "Installing Service Binding Controller"
echo "**************************************"

kubectl apply -f "${DEP_DIR}/service-bindings-0.7.1.yaml"
