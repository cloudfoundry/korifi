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

tmp="$(mktemp -d)"
trap "rm -rf $tmp" EXIT
if [[ -z "${E2E_USER_NAME:=}" ]]; then
  export E2E_USER_NAME="e2e-cert-user"
  createCert "$E2E_USER_NAME" "$tmp/key.pem" "$tmp/cert.pem"

  export E2E_USER_PEM="$(cat $tmp/cert.pem $tmp/key.pem | base64 | tr -d "\n\r")"
fi
if [[ -z "${E2E_LONGCERT_USER_NAME:=}" ]]; then
  export E2E_LONGCERT_USER_NAME="e2e-longcert-user"
  createCert "$E2E_LONGCERT_USER_NAME" "$tmp/longkey.pem" "$tmp/longcert.pem" "365"

  export E2E_LONGCERT_USER_PEM="$(cat $tmp/longcert.pem $tmp/longkey.pem | base64 | tr -d "\n\r")"
fi

if [[ -z "${E2E_SERVICE_ACCOUNT:=}" ]]; then
  export E2E_SERVICE_ACCOUNT="e2e-service-account"
  kubectl delete serviceaccount --ignore-not-found=true -n "$ROOT_NAMESPACE" "$E2E_SERVICE_ACCOUNT" &>/dev/null
  kubectl create serviceaccount -n "$ROOT_NAMESPACE" "$E2E_SERVICE_ACCOUNT"

  token=""
  while [ -z "$token" ]; do
    token="$(getToken "e2e-service-account" "$ROOT_NAMESPACE")"
    sleep 0.5
  done

  export E2E_SERVICE_ACCOUNT_TOKEN="$token"
fi

if [[ -z "${CF_ADMIN_CERT:=}" ]]; then
  createCert "cf-admin" "$tmp/cf-admin-key.pem" "$tmp/cf-admin-cert.pem"
  CF_ADMIN_KEY="$(base64 $tmp/cf-admin-key.pem | tr -d "\n\r")"
  CF_ADMIN_CERT="$(base64 $tmp/cf-admin-cert.pem | tr -d "\n\r")"
  export CF_ADMIN_CERT CF_ADMIN_KEY
fi

export CLUSTER_VERSION_MINOR="$(kubectl version -ojson | jq -r .serverVersion.minor)"
export CLUSTER_VERSION_MAJOR="$(kubectl version -ojson | jq -r .serverVersion.major)"
