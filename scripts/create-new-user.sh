#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/common.sh"

if [[ $# -ne 1 ]]; then
  cat <<EOF >&2
Usage:
  $(basename "$0") <username>

EOF
  exit 1
fi

username="$1"
tmp="$(mktemp -d)"
trap "rm -rf $tmp" EXIT

priv_key_file="$tmp/key.pem"
csr_file="$tmp/csr.pem"
cert_file="$tmp/cert.pem"
csr_name="$(echo ${RANDOM} | shasum | head -c 40)"

openssl req -new -newkey rsa:4096 \
  -keyout "${priv_key_file}" \
  -out "${csr_file}" \
  -nodes \
  -subj "/CN=${username}"

cat <<EOF | kubectl create -f -
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: ${csr_name}
spec:
  signerName: "kubernetes.io/kube-apiserver-client"
  request: "$(base64 "${csr_file}" | tr -d '\n')"
  usages:
  - client auth
EOF

kubectl certificate approve "${csr_name}"

retry [ -n "$(kubectl get csr "${csr_name}" -o jsonpath='{.status.certificate}')" ]
kubectl get csr "${csr_name}" -o jsonpath='{.status.certificate}' | base64 --decode >"${cert_file}"

kubectl config set-credentials "${username}" --client-certificate="${cert_file}" --client-key="${priv_key_file}" --embed-certs

cat <<EOF

Use "cf set-space-role ${username} ORG SPACE SpaceDeveloper" to grant this user permissions in a space.
EOF
