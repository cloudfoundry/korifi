#!/usr/bin/env bash

set -euo pipefail

createCert() {
  username=${1:-}
  priv_key_file=${2:-}
  cert_file=${3:-}
  days=${4:-5}
  csr_file="$(mktemp)"
  trap "rm -f $csr_file" EXIT
  csr_name="$(echo ${RANDOM} | shasum | head -c 40)"

  openssl req -new -newkey rsa:4096 \
    -keyout "${priv_key_file}" \
    -out "${csr_file}" \
    -nodes \
    -subj "/CN=${username}" 2>/dev/null

  cat <<EOF | kubectl create -f -
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: ${csr_name}
spec:
  signerName: "kubernetes.io/kube-apiserver-client"
  request: "$(cat "${csr_file}" | base64 | tr -d "\n\r")"
  expirationSeconds: $((days * 24 * 60 * 60))
  usages:
  - client auth
EOF

  kubectl certificate approve "${csr_name}"
  kubectl wait --for=condition=Approved "csr/${csr_name}"
  cert=
  while [[ -z "$cert" ]]; do
    cert="$(kubectl get csr "${csr_name}" -o jsonpath='{.status.certificate}')"
  done

  base64 --decode <<<$cert >$cert_file
  kubectl delete csr "${csr_name}"
}

main() {
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

  createCert $username $tmp/key.pem $tmp/cert.pem

  kubectl config set-credentials \
    "${username}" \
    --client-certificate="$tmp/cert.pem" \
    --client-key="$tmp/key.pem" \
    --embed-certs

  cat <<EOF

Use "cf set-space-role ${username} ORG SPACE SpaceDeveloper" to grant this user permissions in a space.
EOF
}

main $@
