#!/bin/bash

set -eu

echo "Will now generate tls.ca tls.crt and tls.key files"

mkdir -p keys
trap 'rm -rf keys' EXIT

readonly SYSTEM_NAMESPACE=eirini-controller
otherDNS=$1

pushd keys
{
  kubectl create namespace "$SYSTEM_NAMESPACE" || true

  openssl req -x509 -newkey rsa:4096 -keyout tls.key -out tls.crt -nodes -subj '/CN=localhost' -addext "subjectAltName = DNS:$otherDNS, DNS:$otherDNS.cluster.local" -days 365

  for secret_name in eirini-webhooks-certs; do
    if kubectl -n "$SYSTEM_NAMESPACE" get secret "$secret_name" >/dev/null 2>&1; then
      kubectl delete secret -n "$SYSTEM_NAMESPACE" "$secret_name"
    fi
    echo "Creating the $secret_name secret in your kubernetes cluster"
    kubectl create secret -n "$SYSTEM_NAMESPACE" generic "$secret_name" --from-file=tls.crt=./tls.crt --from-file=tls.ca=./tls.crt --from-file=tls.key=./tls.key
  done

  echo "Done!"
}
popd
