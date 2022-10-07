#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "${SCRIPT_DIR}/common.sh"

function getTestDir() {
  for arg in "$@"; do
    if [[ -d "${arg}" ]]; then
      echo "${arg}"
      return
    fi
  done
  echo "."
}

ENVTEST_ASSETS_DIR="${SCRIPT_DIR}/../testbin"
mkdir -p "${ENVTEST_ASSETS_DIR}"

extra_args=()
if [[ -n "${GINKGO_NODES:-}" ]]; then
  extra_args+=("--procs=${GINKGO_NODES}")
fi

if ! grep -q e2e <(echo "$@"); then
  grepFlags="-sq"

  if [[ -z "${NON_RECURSIVE_TEST:-}" ]]; then
    grepFlags+="r"
  fi

  if grep "${grepFlags}" sigs.k8s.io/controller-runtime/pkg/envtest -- "$(getTestDir "$@")"/*; then
    go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
    source <(setup-envtest use -p env --bin-dir "${ENVTEST_ASSETS_DIR}")
  fi

  extra_args+=("--skip-package=e2e" "--coverprofile=cover.out" "--coverpkg=code.cloudfoundry.org/korifi/...")
else
  export ROOT_NAMESPACE="${ROOT_NAMESPACE:-cf}"
  export APP_FQDN="${APP_FQDN:-vcap.me}"
  export KUBECONFIG="${KUBECONFIG:-${HOME}/kube/e2e.yml}"
  export API_SERVER_ROOT="${API_SERVER_ROOT:-https://localhost}"

  if [ -z "${SKIP_DEPLOY:-}" ]; then
    "${SCRIPT_DIR}/deploy-on-kind.sh" -l e2e
  fi

  # creates user keys/certs and service accounts and exports vars for them
  source "$SCRIPT_DIR/account-creation.sh" "${SCRIPT_DIR}"

  extra_args+=("--slow-spec-threshold=30s")

  echo "waiting for ClusterBuilder to be ready..."
  kubectl wait --for=condition=ready clusterbuilder --all=true --timeout=15m
fi

if [[ -z "${NON_RECURSIVE_TEST:-}" ]]; then
  extra_args+=("-r")
fi

if [[ -n "${UNTIL_IT_FAILS:-}" ]]; then
  extra_args+=("--until-it-fails")
fi

if [[ -n "${SEED:-}" ]]; then
  extra_args+=("--seed=${SEED}")
fi

if [[ -z "${NO_RACE:-}" ]]; then
  extra_args+=("--race")
fi

ginkgo -p --randomize-all --randomize-suites "${extra_args[@]}" $@
