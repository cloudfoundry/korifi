#!/usr/bin/env bash

retry() {
  until $@; do
    echo -n .
    sleep 1
  done
  echo
}

createCert() {
  username=${1:-}
  priv_key_file=${2:-}
  cert_file=${3:-}
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
  request: "$(base64 "${csr_file}" | tr -d "\n\r")"
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
