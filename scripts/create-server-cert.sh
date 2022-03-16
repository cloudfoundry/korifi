#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
SERVICE=cf-k8s-api-svc
NAMESPACE=cf-k8s-api-system
SECRET_NAME=server-cert

TMPDIR="$(mktemp -d)"
trap "rm -rf $TMPDIR" EXIT

openssl genrsa -out "$TMPDIR/server.key" 2048

openssl req -new -key "$TMPDIR/server.key" -subj "/CN=${SERVICE}.${NAMESPACE}.svc" -out "$TMPDIR/server.csr" -config <(
  cat <<EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name
[req_distinguished_name]
[ v3_req ]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = ${SERVICE}
DNS.2 = ${SERVICE}.${NAMESPACE}
DNS.3 = ${SERVICE}.${NAMESPACE}.svc
DNS.4 = ${SERVICE}.${NAMESPACE}.svc.cluster.local
EOF
)

CSR_NAME=server-csr
kubectl delete certificatesigningrequest $CSR_NAME || true
cat <<EOF | kubectl create -f -
apiVersion: certificates.k8s.io/v1beta1
kind: CertificateSigningRequest
metadata:
  name: ${CSR_NAME}
spec:
  groups:
  - system:authenticated
  request: $(cat $TMPDIR/server.csr | base64 | tr -d '\n')
  usages:
  - digital signature
  - key encipherment
  - server auth
EOF
kubectl certificate approve "$CSR_NAME"

sleep 2
serverCert=$(kubectl get csr ${CSR_NAME} -o jsonpath='{.status.certificate}')

echo "$serverCert" | openssl base64 -d -A -out "$TMPDIR/server.crt"

kubectl config view --raw --minify --flatten -o jsonpath='{.clusters[].cluster.certificate-authority-data}' | base64 -d >"$TMPDIR/server.ca"

kubectl create namespace "$NAMESPACE" || true
kubectl delete --namespace "$NAMESPACE" secret ${SECRET_NAME} || true
kubectl create secret generic ${SECRET_NAME} \
  --namespace ${NAMESPACE} \
  --from-file=server.key=${TMPDIR}/server.key \
  --from-file=server.crt=${TMPDIR}/server.crt \
  --from-file=server.ca=${TMPDIR}/server.ca
