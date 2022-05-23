#!/bin/bash

if [[ -n "$1" ]]; then
  SCRIPT_DIR="$1"
fi

source "$SCRIPT_DIR/common.sh"

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
fi

if [[ -z "$E2E_SERVICE_ACCOUNT_TOKEN" ]]; then
  E2E_SERVICE_ACCOUNT_TOKEN_NAME="${E2E_SERVICE_ACCOUNT}-token"
  kubectl delete secret --ignore-not-found=true -n "$ROOT_NAMESPACE" "$E2E_SERVICE_ACCOUNT_TOKEN_NAME" &>/dev/null
  kubectl apply -f - <<TOKEN_SECRET
    apiVersion: v1
    kind: Secret
    metadata:
      name: "$E2E_SERVICE_ACCOUNT_TOKEN_NAME"
      namespace: "$ROOT_NAMESPACE"
      annotations:
        kubernetes.io/service-account.name: "$E2E_SERVICE_ACCOUNT"
    type: kubernetes.io/service-account-token
    data:
TOKEN_SECRET

  token=""
  while [ -z "$token" ]; do
    token="$(kubectl get secrets -n "$ROOT_NAMESPACE" "$E2E_SERVICE_ACCOUNT_TOKEN_NAME" -ojsonpath='{.data.token}' | base64 -d)"
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
