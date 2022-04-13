#!/bin/bash

if [[ -n "$1" ]]; then
  SCRIPT_DIR="$1"
fi

source "$SCRIPT_DIR/common.sh"

getToken() {
  local name namespace secret
  name=${1:?}
  namespace=${2:?}
  secretName="$(kubectl get serviceaccounts -n "$namespace" "$name" -ojsonpath='{.secrets[0].name}')"
  if [[ -n "$secretName" ]]; then
    kubectl get secrets -n "$namespace" "$secretName" -ojsonpath='{.data.token}' | base64 -d
  fi
}

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
if [[ -z "${E2E_USER_NAMES:=}" ]]; then
  E2E_USER_PEMS=
  for n in $(seq 1 $usersToCreate); do
    createCert "e2e-cert-user-$n" "$tmp/key-$n.pem" "$tmp/cert-$n.pem" &
  done
  wait

  export E2E_USER_NAMES E2E_USER_PEMS
  for n in $(seq 1 $usersToCreate); do
    E2E_USER_NAMES="$E2E_USER_NAMES e2e-cert-user-$n"
    pem="$(cat $tmp/cert-${n}.pem $tmp/key-${n}.pem | base64 | tr -d "\n\r")"
    E2E_USER_PEMS="$E2E_USER_PEMS $pem"
  done
fi

if [[ -z "${E2E_SERVICE_ACCOUNTS:=}" ]]; then
  E2E_SERVICE_ACCOUNT_TOKENS=
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

if [[ -z "${CF_ADMIN_CERT:=}" ]]; then
  createCert "cf-admin" "$tmp/cf-admin-key.pem" "$tmp/cf-admin-cert.pem"
  CF_ADMIN_KEY="$(base64 $tmp/cf-admin-key.pem | tr -d "\n\r")"
  CF_ADMIN_CERT="$(base64 $tmp/cf-admin-cert.pem | tr -d "\n\r")"
  export CF_ADMIN_CERT CF_ADMIN_KEY
fi
