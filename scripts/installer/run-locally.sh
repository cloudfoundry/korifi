#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT_DIR="${ROOT_DIR}/scripts"
export PATH="${ROOT_DIR}/bin:$PATH"

CLUSTER_NAME="korifi"

RELEASE_VERSION="${1:-}"
DEV_JOB_DEFINITION="$(mktemp)"
HELM_CHART_SOURCE="$ROOT_DIR/helm_chart_source"
mkdir -p "$HELM_CHART_SOURCE"
trap "rm -rf $HELM_CHART_SOURCE $DEV_JOB_DEFINITION" EXIT

# workaround for https://github.com/carvel-dev/kbld/issues/213
# kbld fails with git error messages in languages than other english
export LC_ALL=en_US.UTF-8

function ensure_kind_cluster() {
  if ! kind get clusters | grep -q "$CLUSTER_NAME"; then
    kind create cluster --name "$CLUSTER_NAME" --wait 5m --config="$SCRIPT_DIR/assets/kind-config.yaml"
  fi

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

    export VERSION=$(git describe --tags | awk -F'[.-]' '{$3++; print $1 "." $2 "." $3 "-" $4 "-" $5}' | awk '{print substr($1,2)}')
    yq -i 'with(.; .version=env(VERSION))' "$HELM_CHART_SOURCE/Chart.yaml"
    yq "with(.sources[]; .docker.buildx.rawOptions += [\"--build-arg\", \"version=$VERSION\"])" $kbld_file |
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

function build_installer() {
  pushd "${ROOT_DIR}" >/dev/null
  {
    local kbld_file
    kbld_file="$SCRIPT_DIR/installer/kbld.yaml"

    yq "with(.sources[]; .docker.buildx.rawOptions += [\"--build-arg\", \"HELM_CHART_SOURCE=helm_chart_source\"])" $kbld_file |
      kbld \
        --images-annotation=false \
        -f "${ROOT_DIR}/scripts/installer/install-korifi-kind.yaml" \
        -f - >"$DEV_JOB_DEFINITION"

    awk '/image:/ {print $2}' "$DEV_JOB_DEFINITION" | xargs kind load docker-image --name "$CLUSTER_NAME"
  }
  popd >/dev/null
}

function run_installer() {
  pushd "${ROOT_DIR}" >/dev/null
  {
    kubectl delete --ignore-not-found=true namespace korifi-installer
    kubectl apply -f "$JOB_DEFINITION"
    kubectl wait -n korifi-installer --for=condition=ready pod -l job-name=install-korifi
    kubectl logs -n korifi-installer -l job-name=install-korifi -f
  }
  popd >/dev/null
}

function main() {
  make -C "$ROOT_DIR" bin/yq

  ensure_kind_cluster
  JOB_DEFINITION="https://github.com/cloudfoundry/korifi/releases/download/$RELEASE_VERSION/install-korifi-kind.yaml"
  if [[ "$RELEASE_VERSION" == "" ]]; then
    clone_helm_chart
    build_korifi
    build_installer
    JOB_DEFINITION="$DEV_JOB_DEFINITION"
  fi
  run_installer
}

main
