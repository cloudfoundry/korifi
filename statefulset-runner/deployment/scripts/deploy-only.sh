#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT_DIR="$ROOT_DIR/deployment/scripts"

export KUBECONFIG
KUBECONFIG=${KUBECONFIG:-$HOME/.kube/config}
KUBECONFIG=$(readlink -f "$KUBECONFIG")

export GOOGLE_APPLICATION_CREDENTIALS
GOOGLE_APPLICATION_CREDENTIALS=${GOOGLE_APPLICATION_CREDENTIALS:-""}
if [[ -n $GOOGLE_APPLICATION_CREDENTIALS ]]; then
  GOOGLE_APPLICATION_CREDENTIALS=$(readlink -f "$GOOGLE_APPLICATION_CREDENTIALS")
fi

readonly SYSTEM_NAMESPACE=eirini-controller

readonly HELM_VALUES=${HELM_VALUES:-"$ROOT_DIR/deployment/helm/values.yaml"}

source "$SCRIPT_DIR/helpers/print.sh"

main() {
  print_disclaimer
  generate_secrets
  install_prometheus
  install_eirini_controller "$@"
}

generate_secrets() {
  "$SCRIPT_DIR/generate-secrets.sh" "*.${SYSTEM_NAMESPACE}.svc"
}

install_prometheus() {
  helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
  helm repo update
  helm upgrade prometheus \
    --install prometheus-community/prometheus \
    --namespace "$SYSTEM_NAMESPACE" \
    --wait
}

install_eirini_controller() {
  local resource_validator_ca_bundle env_injector_ca_bundle
  webhook_ca_bundle="$(kubectl get secret -n $SYSTEM_NAMESPACE eirini-webhooks-certs -o jsonpath="{.data['tls\.ca']}")"
  helm upgrade eirini-controller \
    --install "$ROOT_DIR/deployment/helm" \
    --namespace "$SYSTEM_NAMESPACE" \
    --values "$HELM_VALUES" \
    --values "$SCRIPT_DIR/assets/value-overrides.yaml" \
    --set "webhooks.ca_bundle=$webhook_ca_bundle" \
    --wait \
    "$@"
}

main "$@"
