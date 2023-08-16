#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ENVTEST_ASSETS_DIR="${SCRIPT_DIR}/../testbin"
mkdir -p "${ENVTEST_ASSETS_DIR}"
extra_args=()

function getTestDir() {
  for arg in "$@"; do
    if [[ -d "${arg}" ]]; then
      echo "${arg}"
      return
    fi
  done
  echo "."
}

function deploy_korifi() {
  if [ -z "${SKIP_DEPLOY:-}" ]; then
    "${SCRIPT_DIR}/deploy-on-kind.sh" e2e
  fi

  echo "waiting for ClusterBuilder to be ready..."
  kubectl wait --for=condition=ready clusterbuilder --all=true --timeout=15m
}

function configure_e2e_tests() {
  export ROOT_NAMESPACE="${ROOT_NAMESPACE:-cf}"
  export APP_FQDN="${APP_FQDN:-apps-127-0-0-1.nip.io}"
  export API_SERVER_ROOT="${API_SERVER_ROOT:-https://localhost}"

  deploy_korifi

  extra_args+=("--poll-progress-after=3m30s")
}

function configure_crd_tests() {
  export NO_PARALLEL=true
  export CRDS_TEST_API_ENDPOINT="${CRDS_TEST_API_ENDPOINT:-https://localhost}"
  export CRDS_TEST_SKIP_SSL="${CRDS_TEST_SKIP_SSL:-true}"

  deploy_korifi
}

function configure_smoke_tests() {
  export NO_PARALLEL=true
  export SMOKE_TEST_USER="${SMOKE_TEST_USER:-cf-admin}"
  export SMOKE_TEST_APPS_DOMAIN="${SMOKE_TEST_APPS_DOMAIN:-apps-127-0-0-1.nip.io}"
  export SMOKE_TEST_APP_ROUTE_PROTOCOL="${SMOKE_TEST_APP_ROUTE_PROTOCOL:-https}"
  export SMOKE_TEST_API_ENDPOINT="${SMOKE_TEST_API_ENDPOINT:-https://localhost}"
  export SMOKE_TEST_SKIP_SSL="${SMOKE_TEST_SKIP_SSL:-true}"

  deploy_korifi
}

function configure_non_e2e_tests() {
  grepFlags="-sq"

  if [[ -z "${NON_RECURSIVE_TEST:-}" ]]; then
    grepFlags+="r"
  fi

  if grep "${grepFlags}" sigs.k8s.io/controller-runtime/pkg/envtest -- "$(getTestDir "$@")"/*; then
    go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
    source <(setup-envtest use -p env --bin-dir "${ENVTEST_ASSETS_DIR}")
  fi

  extra_args+=("--poll-progress-after=60s" "--skip-package=e2e")
}

function run_ginkgo() {
  if [[ -n "${GINKGO_NODES:-}" ]]; then
    extra_args+=("--procs=${GINKGO_NODES}")
  fi

  if [[ -z "${NO_COVERAGE:-}" ]]; then
    extra_args+=("--coverprofile=cover.out" "--coverpkg=code.cloudfoundry.org/korifi/...")
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

  if [[ -z "${NO_PARALLEL:-}" ]]; then
    extra_args+=("-p")
  fi

  go run github.com/onsi/ginkgo/v2/ginkgo --randomize-all --randomize-suites "${extra_args[@]}" $@
}

function main() {

  if grep -q "tests/e2e" <(echo "$@"); then
    configure_e2e_tests $@
  elif grep -q "tests/crds" <(echo "$@"); then
    configure_crd_tests $@
  elif grep -q "tests/smoke" <(echo "$@"); then
    configure_smoke_tests $@
  else
    configure_non_e2e_tests $@
  fi

  run_ginkgo $@
}

main $@
