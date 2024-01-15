#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT_DIR="${ROOT_DIR}/scripts"

CLUSTER_NAME="installer"

JOB_DEFINITION="$(mktemp)"
HELM_CHART_SOURCE="$ROOT_DIR/helm_chart_source"
mkdir -p "$HELM_CHART_SOURCE"
trap "rm -rf $HELM_CHART_SOURCE $JOB_DEFINITION" EXIT

# workaround for https://github.com/carvel-dev/kbld/issues/213
# kbld fails with git error messages in languages than other english
export LC_ALL=en_US.UTF-8

function install_yq() {
  GOBIN="${ROOT_DIR}/bin" go install github.com/mikefarah/yq/v4@latest
  YQ="${ROOT_DIR}/bin/yq"
}

function ensure_kind_cluster() {
  kind delete cluster --name "$CLUSTER_NAME"
  kind create cluster --name "$CLUSTER_NAME" --wait 5m --config="$SCRIPT_DIR/assets/kind-config.yaml"

  kind export kubeconfig --name "$CLUSTER_NAME"
}

function clone_helm_chart() {
  echo "Cloning helm chart in $HELM_CHART_SOURCE"
  cp -a "$ROOT_DIR"/helm/korifi/* "$HELM_CHART_SOURCE"
}

function build_korifi() {
  pushd "${ROOT_DIR}" >/dev/null
  {
    echo "Building korifi values file..."

    make generate manifests

    local kbld_file values_file
    kbld_file="$SCRIPT_DIR/assets/korifi-kbld.yml"
    values_file="$HELM_CHART_SOURCE/values.yaml"

    "$YQ" "with(.sources[]; .docker.buildx.rawOptions += [\"--build-arg\", \"version=dev\"])" $kbld_file |
      kbld \
        --images-annotation=false \
        -f "${ROOT_DIR}/helm/korifi/values.yaml" \
        -f - >"$values_file"

    awk '/image:/ {print $2}' "$values_file" | while read -r img; do
      kind load docker-image --name "$CLUSTER_NAME" "$img"
    done
  }
  popd >/dev/null
}

run_installer() {
  pushd "${ROOT_DIR}" >/dev/null
  {
    local kbld_file
    kbld_file="$SCRIPT_DIR/installer/kbld.yaml"

    "$YQ" "with(.sources[]; .docker.buildx.rawOptions += [\"--build-arg\", \"HELM_CHART_SOURCE=helm_chart_source\"])" $kbld_file |
      kbld \
        --images-annotation=false \
        -f "${ROOT_DIR}/scripts/installer/install-korifi-kind.yaml" \
        -f - >"$JOB_DEFINITION"

    awk '/image:/ {print $2}' "$JOB_DEFINITION" | xargs kind load docker-image --name "$CLUSTER_NAME"

    kubectl apply -f "$JOB_DEFINITION"
    kubectl wait -n korifi-installer --for=condition=ready pod -l job-name=install-korifi
    kubectl logs -n korifi-installer -l job-name=install-korifi -f
  }
  popd >/dev/null
}

function main() {
  install_yq
  ensure_kind_cluster
  clone_helm_chart
  build_korifi
  run_installer
}

main "$@"
