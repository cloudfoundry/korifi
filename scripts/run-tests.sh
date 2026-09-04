#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="${SCRIPT_DIR}/.."
PATH="$PATH":"$ROOT_DIR/bin"
ENVTEST_ASSETS_DIR="${ROOT_DIR}/testbin"
mkdir -p "${ENVTEST_ASSETS_DIR}"
extra_args=()

function deploy_korifi() {
  if [ -z "${SKIP_DEPLOY:-}" ]; then
    "${SCRIPT_DIR}/deploy-on-kind.sh" e2e
  fi

  echo "waiting for ClusterBuilder to be ready..."
  kubectl wait --for=condition=ready clusterbuilder --all=true --timeout=15m
}

function configure_e2e_tests() {
  export API_SERVER_ROOT="${API_SERVER_ROOT:-https://localhost}"
  export APP_FQDN="${APP_FQDN:-apps-127-0-0-1.nip.io}"
  export ROOT_NAMESPACE="${ROOT_NAMESPACE:-cf}"

  deploy_korifi

  extra_args+=("--poll-progress-after=3m30s")
}

function configure_crd_tests() {
  export API_SERVER_ROOT="${API_SERVER_ROOT:-https://localhost}"

  deploy_korifi
}

function configure_smoke_tests() {
  export API_SERVER_ROOT="${API_SERVER_ROOT:-https://localhost}"
  export APP_FQDN="${APP_FQDN:-apps-127-0-0-1.nip.io}"

  deploy_korifi
}

function configure_non_e2e_tests() {
  make -C "$ROOT_DIR" bin/setup-envtest
  source <("$ROOT_DIR/bin/setup-envtest" use -p env --bin-dir "${ENVTEST_ASSETS_DIR}")

  extra_args+=("--poll-progress-after=60s" "--skip-package=e2e")
}

function cover_enabled() {
  # Opt in with COVER=1 locally; CI enables by default (GitHub Actions sets CI=true).
  # Opt out with NO_COVER=1.
  if [[ -n "${NO_COVER:-}" ]]; then
    return 1
  fi
  [[ -n "${COVER:-}" || -n "${CI:-}" ]]
}

function print_coverage() {
  local profile="${1:-cover.out}"
  if [[ ! -f "${profile}" ]]; then
    echo "Coverage profile ${profile} not found; skipping summary."
    return 0
  fi

  local summary total pct job_label
  summary="$(go tool cover -func="${profile}")"
  total="$(echo "${summary}" | tail -n1)"
  pct="$(echo "${total}" | awk '{print $NF}')"
  job_label="${GITHUB_JOB:-$(basename "$(pwd)")}"

  # Full per-function table is collapsible in the job log; total stays visible.
  echo "::group::Coverage summary"
  echo "${summary}"
  echo "::endgroup::"
  echo "${total}"

  # Exported for the workflow rollup job (needs.*.outputs.coverage).
  if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
    echo "coverage=${pct}" >>"${GITHUB_OUTPUT}"
  fi

  # Per-job drill-down on the run Summary page (rollup table is separate).
  if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
    {
      echo "### Coverage: \`${job_label}\`"
      echo ""
      echo "**${pct}** statement coverage"
      echo ""
      echo "<details>"
      echo "<summary>Per-function coverage</summary>"
      echo ""
      echo '```'
      echo "${summary}"
      echo '```'
      echo ""
      echo "</details>"
      echo ""
    } >>"${GITHUB_STEP_SUMMARY}"
  fi
}

function run_ginkgo() {
  if [[ -n "${GINKGO_NODES:-}" ]]; then
    extra_args+=("--procs=${GINKGO_NODES}")
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

  if [[ -z "${KEEP_GOING:-}" ]]; then
    extra_args+=("--keep-going")
  fi

  if cover_enabled; then
    # atomic is required when --race is enabled; safe without race too.
    extra_args+=("--cover" "--covermode=atomic" "--coverprofile=cover.out")
  fi

  go run github.com/onsi/ginkgo/v2/ginkgo --output-interceptor-mode=none --randomize-all --randomize-suites "${extra_args[@]}" $@

  if cover_enabled; then
    print_coverage cover.out
  fi
}

function main() {
  make bin/controller-gen

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
