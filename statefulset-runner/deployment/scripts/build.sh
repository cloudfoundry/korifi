#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
DEPLOYMENT_DIR="$ROOT_DIR/deployment"
SCRIPT_DIR="$DEPLOYMENT_DIR/scripts"

source "$SCRIPT_DIR/helpers/print.sh"

main() {
  print_disclaimer
  build_eirini_controller "$@"
}

build_eirini_controller() {
  pushd "$ROOT_DIR"
  {
    kbld -f "$SCRIPT_DIR"/assets/kbld.yaml -f "$DEPLOYMENT_DIR"/helm/values-template.yaml >"$DEPLOYMENT_DIR"/helm/values.yaml
  }
  popd
}

main "$@"
