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
  expirationSeconds: $((days*24*60*60))
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

function create_tls_secret() {
  local secret_name=${1:?}
  local secret_namespace=${2:?}
  local tls_cn=${3:?}

  tmp_dir=$(mktemp -d -t cf-tls-XXXXXX)
  trap "rm -rf $tmp_dir" RETURN

  opensslVersion="$(openssl version)"
  if [[ $opensslVersion =~ ^OpenSSL ]]; then
    openssl req -x509 -newkey rsa:4096 \
      -keyout ${tmp_dir}/tls.key \
      -out ${tmp_dir}/tls.crt \
      -nodes \
      -subj "/CN=${tls_cn}" \
      -addext "subjectAltName = DNS:${tls_cn}" \
      -days 365 2>/dev/null
  elif [[ $opensslVersion =~ ^LibreSSL ]]; then
    openssl req -x509 -newkey rsa:4096 \
      -keyout ${tmp_dir}/tls.key \
      -out ${tmp_dir}/tls.crt \
      -nodes \
      -subj "/CN=${tls_cn}" \
      -extensions SAN -config <(cat /etc/ssl/openssl.cnf <(printf "[ SAN ]\nsubjectAltName='DNS:${tls_cn}'")) \
      -days 365 2>/dev/null
  else
    echo "OpenSSL $(openssl version) not supported"
    exit 1
  fi

  cat <<EOF >${tmp_dir}/kustomization.yml
secretGenerator:
- name: ${secret_name}
  namespace: ${secret_namespace}
  files:
  - tls.crt=tls.crt
  - tls.key=tls.key
  type: "kubernetes.io/tls"
generatorOptions:
  disableNameSuffixHash: true
EOF

  kubectl apply -k $tmp_dir
}
