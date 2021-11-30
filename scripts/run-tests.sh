#!/usr/bin/env bash

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

ENVTEST_ASSETS_DIR="${SCRIPT_DIR}/../testbin"
mkdir -p "${ENVTEST_ASSETS_DIR}"

go install github.com/onsi/ginkgo/ginkgo

extra_args=()
if ! egrep -q e2e <(echo "$@"); then
  test -f "${ENVTEST_ASSETS_DIR}/setup-envtest.sh" || curl -sSLo "${ENVTEST_ASSETS_DIR}/setup-envtest.sh" https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.8.3/hack/setup-envtest.sh
  source "${ENVTEST_ASSETS_DIR}/setup-envtest.sh"
  fetch_envtest_tools "${ENVTEST_ASSETS_DIR}"
  setup_envtest_env "${ENVTEST_ASSETS_DIR}"
  extra_args+=("--skip-package=e2e" "--coverprofile=cover.out" "--coverpkg=code.cloudfoundry.org/cf-k8s-controllers/...")
else
  if [ -z "${SKIP_DEPLOY}" ]; then
    "${SCRIPT_DIR}/deploy-on-kind.sh" e2e
  fi

  export KUBECONFIG="${HOME}/.kube/e2e.yml"
  export API_SERVER_ROOT=http://localhost
  export ROOT_NAMESPACE=cf

  extra_args+=("--slow-spec-threshold=30s")
fi

if [[ -n "$GINKGO_NODES" ]]; then
  extra_args+=("--procs=${GINKGO_NODES}")
fi

ginkgo -r -p --randomize-all --randomize-suites "${extra_args[@]}" $@
