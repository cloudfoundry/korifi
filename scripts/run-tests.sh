#!/usr/bin/env bash

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/common.sh"

ENVTEST_ASSETS_DIR="${SCRIPT_DIR}/../testbin"
mkdir -p "${ENVTEST_ASSETS_DIR}"

getToken() {
  local name namespace secret
  name=${1:?}
  namespace=${2:?}
  secretName="$(kubectl get serviceaccounts -n "$namespace" "$name" -ojsonpath='{.secrets[0].name}')"
  if [[ -n "$secretName" ]]; then
    kubectl get secrets -n "$namespace" "$secretName" -ojsonpath='{.data.token}' | base64 -d
  fi
}

extra_args=()
if ! egrep -q e2e <(echo "$@"); then
  test -f "${ENVTEST_ASSETS_DIR}/setup-envtest.sh" || curl -sSLo "${ENVTEST_ASSETS_DIR}/setup-envtest.sh" https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.8.3/hack/setup-envtest.sh
  source "${ENVTEST_ASSETS_DIR}/setup-envtest.sh"
  fetch_envtest_tools "${ENVTEST_ASSETS_DIR}"
  setup_envtest_env "${ENVTEST_ASSETS_DIR}"
  extra_args+=("--skip-package=e2e" "--coverprofile=cover.out" "--coverpkg=code.cloudfoundry.org/cf-k8s-controllers/...")

else
  export ROOT_NAMESPACE=cf

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

  pushd "$SCRIPT_DIR/assets/ginkgo_parallel_count"
  {
    usersToCreate="$(ginkgo -p "${extra_args[@]}" | awk '/ParallelNodes:/ { print $2 }' | head -n1)"
  }
  popd

  tmp="$(mktemp -d)"
  trap "rm -rf $tmp" EXIT
  if [[ -z "$E2E_USER_NAMES" ]]; then
    for n in $(seq 1 $usersToCreate); do
      createCert "e2e-cert-user-$n" "$tmp/key-$n.pem" "$tmp/cert-$n.pem" &
    done
    wait

    export E2E_USER_NAMES E2E_USER_PEMS
    for n in $(seq 1 $usersToCreate); do
      E2E_USER_NAMES="$E2E_USER_NAMES e2e-cert-user-$n"
      pem="$(cat $tmp/cert-${n}.pem $tmp/key-${n}.pem | base64 -w0)"
      E2E_USER_PEMS="$E2E_USER_PEMS $pem"
    done
  fi

  if [[ -z "$E2E_SERVICE_ACCOUNTS" ]]; then
    for n in $(seq 1 $usersToCreate); do
      (
        kubectl delete serviceaccount -n "$ROOT_NAMESPACE" "e2e-service-account-$n" &>/dev/null || true
        kubectl create serviceaccount -n "$ROOT_NAMESPACE" "e2e-service-account-$n"
      ) &
    done
    wait

    export E2E_SERVICE_ACCOUNTS E2E_SERVICE_ACCOUNT_TOKENS
    for n in $(seq 1 $usersToCreate); do
      E2E_SERVICE_ACCOUNTS="$E2E_SERVICE_ACCOUNTS e2e-service-account-$n"
      token=""
      while [ -z "$token" ]; do
        token="$(getToken "e2e-service-account-$n" "$ROOT_NAMESPACE")"
        sleep 0.5
      done
      E2E_SERVICE_ACCOUNT_TOKENS="$E2E_SERVICE_ACCOUNT_TOKENS $token"
    done
  fi

  createCert "cf-admin" "$tmp/cf-admin-key.pem" "$tmp/cf-admin-cert.pem"
  CF_ADMIN_KEY="$(base64 -w0 $tmp/cf-admin-key.pem)"
  CF_ADMIN_CERT="$(base64 -w0 $tmp/cf-admin-cert.pem)"
  export CF_ADMIN_CERT CF_ADMIN_KEY

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
