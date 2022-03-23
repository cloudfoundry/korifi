#!/usr/bin/env bash

set -euo pipefail

echo "Will now generate tls.ca tls.crt and tls.key files..."

keys="$(mktemp -d)"
trap 'rm -rf "${keys}"' EXIT

readonly EIRINI_NAMESPACE=eirini-controller
otherDNS="$1"

kubectl create namespace "${EIRINI_NAMESPACE}" || true

if [[ "$(openssl version | awk '{ print $1 }')" == "OpenSSL" ]]; then
  openssl req -x509 -newkey rsa:4096 \
    -keyout "${keys}/tls.key" \
    -out "${keys}/tls.crt" \
    -nodes \
    -subj '/CN=localhost' \
    -addext "subjectAltName = DNS:${otherDNS}, DNS:${otherDNS}.cluster.local" \
    -days 365
else
  openssl req -x509 -newkey rsa:4096 \
    -keyout "${keys}/tls.key" \
    -out "${keys}/tls.crt" \
    -nodes \
    -subj '/CN=localhost' \
    -extensions SAN -config <(cat /etc/ssl/openssl.cnf <(printf "[ SAN ]\nsubjectAltName='DNS:${otherDNS}, DNS:${otherDNS}.cluster.local'")) \
    -days 365
fi

for secret_name in eirini-webhooks-certs; do
  if kubectl -n "${EIRINI_NAMESPACE}" get secret "${secret_name}" >/dev/null 2>&1; then
    kubectl delete secret "${secret_name}" \
      -n "${EIRINI_NAMESPACE}"
  fi

  echo "Creating the ${secret_name} secret in your kubernetes cluster..."
  kubectl create secret generic "${secret_name}" \
    -n "${EIRINI_NAMESPACE}" \
    --from-file=tls.crt="${keys}/tls.crt" \
    --from-file=tls.ca="${keys}/tls.crt" \
    --from-file=tls.key="${keys}/tls.key"
done

echo "Done!"
