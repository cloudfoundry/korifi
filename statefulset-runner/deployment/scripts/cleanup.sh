#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
readonly SYSTEM_NAMESPACE=eirini-controller

source "$SCRIPT_DIR/helpers/print.sh"

delete_eirini_controller() {
  helm --namespace "$SYSTEM_NAMESPACE" delete eirini-controller || true
  helm --namespace "$SYSTEM_NAMESPACE" delete prometheus || true
  kubectl delete namespace "$SYSTEM_NAMESPACE" --wait || true
}

main() {
  print_disclaimer
  delete_eirini_controller
}

main
