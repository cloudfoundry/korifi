#!/usr/bin/env bash

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/common.sh"

ENVTEST_ASSETS_DIR="${SCRIPT_DIR}/../testbin"
mkdir -p "${ENVTEST_ASSETS_DIR}"

extra_args=()
if ! egrep -q e2e <(echo "$@"); then
  test -f "${ENVTEST_ASSETS_DIR}/setup-envtest.sh" || curl -sSLo "${ENVTEST_ASSETS_DIR}/setup-envtest.sh" https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.8.3/hack/setup-envtest.sh
  source "${ENVTEST_ASSETS_DIR}/setup-envtest.sh"
  fetch_envtest_tools "${ENVTEST_ASSETS_DIR}"
  setup_envtest_env "${ENVTEST_ASSETS_DIR}"
  extra_args+=("--skip-package=e2e" "--coverprofile=cover.out" "--coverpkg=code.cloudfoundry.org/korifi/...")

else
  export ROOT_NAMESPACE="${ROOT_NAMESPACE:-cf}"

  if [[ -z "${APP_FQDN}" ]]; then
    export APP_FQDN=vcap.me
  fi

  export KUBECONFIG="${KUBECONFIG:-$HOME/kube/e2e.yml}"

  if [ -z "${SKIP_DEPLOY}" ]; then
    "${SCRIPT_DIR}/deploy-on-kind.sh" -l -d e2e
  fi

  if [[ -z "${API_SERVER_ROOT}" ]]; then
    export API_SERVER_ROOT=https://localhost
  fi

  if [[ -n "$GINKGO_NODES" ]]; then
    extra_args+=("--procs=${GINKGO_NODES}")
  fi

  # creates user keys/certs and service accounts and exports vars for them
  source "$SCRIPT_DIR/account-creation.sh" "$SCRIPT_DIR"

  extra_args+=("--slow-spec-threshold=30s")
fi

if [[ -z "$NON_RECURSIVE_TEST" ]]; then
  extra_args+=("-r")
fi

if [[ -n "$UNTIL_IT_FAILS" ]]; then
  extra_args+=("--until-it-fails")
fi

if [[ -n "$SEED" ]]; then
  extra_args+=("--seed=${SEED}")
fi

ginkgo --race -p --randomize-all --randomize-suites "${extra_args[@]}" $@
