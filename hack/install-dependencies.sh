#!/usr/bin/env bash

set -e

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

if [[ $# -lt 1 ]]; then
  usage_text >&2
fi

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

# Install Cert Manager
kubectl apply -f dependencies/cert-manager.yaml

# For GCR with a json key, DOCKER_USERNAME is `_json_key`
DOCKER_USERNAME=${DOCKER_USERNAME:-"_json_key"}
DOCKER_PASSWORD=${DOCKER_PASSWORD:-"$(cat $GCP_SERVICE_ACCOUNT_JSON_FILE)"}
DOCKER_SERVER=${DOCKER_SERVER:-"gcr.io"}

# Kpack
kubectl create secret docker-registry kpack-registry-credentials \
    --docker-username=$DOCKER_USERNAME --docker-password="$DOCKER_PASSWORD" --docker-server=$DOCKER_SERVER --namespace default || true
# kubectl create secret docker-registry kpack-registry-credentials --docker-username="_json_key" --docker-password="$(cat /home/birdrock/workspace/credentials/cf-relint-greengrass-2826975617b2.json)" --docker-server=gcr.io --namespace default


kubectl apply -f dependencies/kpack/release-0.3.1.yaml
kubectl apply -f dependencies/kpack/serviceaccount.yaml \
    -f dependencies/kpack/stack.yaml \
    -f dependencies/kpack/store.yaml \
    -f dependencies/kpack/cluster-builder.yaml

echo "******************************"
echo "Installed and configured Kpack"
echo "******************************"

kubectl apply -f dependencies/contour-1.18.1.yaml
