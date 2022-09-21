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
  days=${4:-5}
  csr_file="$(mktemp)"
  trap "rm -f $csr_file" EXIT
  csr_name="$(echo ${RANDOM} | shasum | head -c 40)"

  openssl req -new -newkey rsa:4096 \
    -keyout "${priv_key_file}" \
    -out "${csr_file}" \
    -nodes \
    -subj "/CN=${username}" 2>/dev/null

  # note: we need 'validate=false' here in order to install on k8s clusters with
  #  version <= 1.21, which don't support expirationSeconds. Those environments will
  #  end up with long-lived certificates.
  cat <<EOF | kubectl create --validate=false -f -
apiVersion: certificates.k8s.io/v1
kind: CertificateSigningRequest
metadata:
  name: ${csr_name}
spec:
  signerName: "kubernetes.io/kube-apiserver-client"
  request: "$(base64 "${csr_file}" | tr -d "\n\r")"
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

function create_tls_cert() {
  local name=${1:?}
  local component=${2:?}
  local cn=${3:?}

  cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: $name
  namespace: $component-system
spec:
  commonName: $cn
  dnsNames:
  - $cn
  issuerRef:
    kind: Issuer
    name: $component-selfsigned-issuer
  secretName: $name
EOF
}
