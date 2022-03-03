#!/usr/bin/env bash

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

ENVTEST_ASSETS_DIR="${SCRIPT_DIR}/../testbin"
mkdir -p "${ENVTEST_ASSETS_DIR}"

extra_args=()
if ! egrep -q e2e <(echo "$@"); then
  test -f "${ENVTEST_ASSETS_DIR}/setup-envtest.sh" || curl -sSLo "${ENVTEST_ASSETS_DIR}/setup-envtest.sh" https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.8.3/hack/setup-envtest.sh
  source "${ENVTEST_ASSETS_DIR}/setup-envtest.sh"
  fetch_envtest_tools "${ENVTEST_ASSETS_DIR}"
  setup_envtest_env "${ENVTEST_ASSETS_DIR}"
  extra_args+=("--skip-package=e2e" "--coverprofile=cover.out" "--coverpkg=code.cloudfoundry.org/cf-k8s-controllers/...")
else
  export KUBECONFIG="${HOME}/.kube/e2e.yml"
  if [ -z "${SKIP_DEPLOY}" ]; then
    "${SCRIPT_DIR}/deploy-on-kind.sh" -l e2e
  fi

  export API_SERVER_ROOT=https://localhost
  export APP_FQDN=vcap.me
  export ROOT_NAMESPACE=cf
  export CF_ADMIN_CERT=$(kubectl config view --raw -o jsonpath='{.users[?(@.name == "cf-admin")].user.client-certificate-data}')
  export CF_ADMIN_KEY=$(kubectl config view --raw -o jsonpath='{.users[?(@.name == "cf-admin")].user.client-key-data}')

  extra_args+=("--slow-spec-threshold=30s")
fi

if [[ -n "$GINKGO_NODES" ]]; then
  extra_args+=("--procs=${GINKGO_NODES}")
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
