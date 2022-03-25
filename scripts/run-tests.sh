#!/usr/bin/env bash

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

ENVTEST_ASSETS_DIR="${SCRIPT_DIR}/../testbin"
mkdir -p "${ENVTEST_ASSETS_DIR}"

getCert() {
  local name
  name=${1:?}
  kubectl config view --raw -o jsonpath='{.users[?(@.name == "'$name'")].user.client-certificate-data}' | base64 -d
}

getKey() {
  local name
  name=${1:?}
  kubectl config view --raw -o jsonpath='{.users[?(@.name == "'$name'")].user.client-key-data}' | base64 -d
}

getToken() {
  local name namespace secret
  name=${1:?}
  namespace=${2:?}
  secret="$(kubectl get serviceaccounts -n "$namespace" "$name" -ojsonpath='{.secrets[0].name}')"
  kubectl get secrets -n "$namespace" "$secret" -ojsonpath='{.data.token}' | base64 -d
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

  export KUBECONFIG="${HOME}/.kube/e2e.yml"
  if [ -z "${SKIP_DEPLOY}" ]; then
    "${SCRIPT_DIR}/deploy-on-kind.sh" -l e2e
  fi

  if [[ -z "${API_SERVER_ROOT}" ]]; then
    export API_SERVER_ROOT=https://localhost
  fi

  if [[ -z "${APP_FQDN}" ]]; then
    export APP_FQDN=vcap.me
  fi

  if [[ -n "$GINKGO_NODES" ]]; then
    extra_args+=("--procs=${GINKGO_NODES}")
  fi

  pushd "$SCRIPT_DIR/assets/ginkgo_parallel_count"
  {
    usersToCreate="$(ginkgo -p "${extra_args[@]}" | awk '/ParallelNodes:/ { print $2 }' | head -n1)"
  }
  popd

  if [[ -z "$E2E_USER_NAMES" ]]; then
    for n in $(seq 1 $usersToCreate); do
      E2E_USER_NAMES="$E2E_USER_NAMES e2e-cert-user-$n"
      "$SCRIPT_DIR/create-new-user.sh" "e2e-cert-user-$n"
      cert="$(getCert "e2e-cert-user-$n")"
      key="$(getKey "e2e-cert-user-$n")"
      pem="$(echo -e "$cert\n$key" | base64 -w0)"
      E2E_USER_PEMS="$E2E_USER_PEMS $pem"
    done
    export E2E_USER_NAMES E2E_USER_PEMS
  fi

  if [[ -z "$E2E_SERVICE_ACCOUNTS" ]]; then
    for n in $(seq 1 $usersToCreate); do
      E2E_SERVICE_ACCOUNTS="$E2E_SERVICE_ACCOUNTS e2e-service-account-$n"
      kubectl delete serviceaccount -n "$ROOT_NAMESPACE" "e2e-service-account-$n" &>/dev/null || true
      kubectl create serviceaccount -n "$ROOT_NAMESPACE" "e2e-service-account-$n"
      token="$(getToken "e2e-service-account-$n" "$ROOT_NAMESPACE")"
      E2E_SERVICE_ACCOUNT_TOKENS="$E2E_SERVICE_ACCOUNT_TOKENS $token"
    done
    export E2E_SERVICE_ACCOUNTS E2E_SERVICE_ACCOUNT_TOKENS
  fi

  CF_ADMIN_CERT="$(getCert cf-admin | base64 -w0)"
  CF_ADMIN_KEY="$(getKey cf-admin | base64 -w0)"
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
