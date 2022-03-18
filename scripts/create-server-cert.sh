#!/bin/bash

set -ex

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
SERVICE=cf-k8s-api-svc
NAMESPACE=cf-k8s-api-system
SECRET_NAME=server-cert

TMPDIR="$(mktemp -d)"
trap "rm -rf $TMPDIR" EXIT

openssl genrsa -out "$TMPDIR/server.key" 2048
openssl req -new \
  -x509 \
  -subj "/C=IT" \
  -key "$TMPDIR/server.key" \
  -out "$TMPDIR/server.crt" \
  -addext "subjectAltName = DNS:${SERVICE}, DNS:${SERVICE}.${NAMESPACE}, DNS:${SERVICE}.${NAMESPACE}.svc, DNS:${SERVICE}.${NAMESPACE}.svc.cluster.local"

openssl x509 -in "$TMPDIR/server.crt" -text -noout

yq e -i ".spec.caBundle = \"$(base64 ${TMPDIR}/server.crt)\"" api/config/base/apiservice.yaml

kubectl create namespace "$NAMESPACE" || true
kubectl delete --namespace "$NAMESPACE" secret ${SECRET_NAME} || true
kubectl create secret generic ${SECRET_NAME} \
  --namespace ${NAMESPACE} \
  --from-file=server.key=${TMPDIR}/server.key \
  --from-file=server.crt=${TMPDIR}/server.crt \
  --from-file=server.ca=${TMPDIR}/server.crt
