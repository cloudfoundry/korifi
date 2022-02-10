#!/usr/bin/env bash

echo "Will now generate tls.ca tls.crt and tls.key files"

keys="$(mktemp -d)"
trap 'rm -rf "${keys}"' EXIT

readonly EIRINI_NAMESPACE=eirini-controller
otherDNS="$1"

pushd "${keys}"
{
  kubectl create namespace "${EIRINI_NAMESPACE}" || true

  openssl req -x509 -newkey rsa:4096 -keyout tls.key -out tls.crt -nodes -subj '/CN=localhost' -addext "subjectAltName = DNS:${otherDNS}, DNS:${otherDNS}.cluster.local" -days 365 ||
    openssl req -x509 -newkey rsa:4096 -keyout tls.key -out tls.crt -nodes -subj '/CN=localhost' -extensions SAN -config <(cat /etc/ssl/openssl.cnf <(printf "[ SAN ]\nsubjectAltName='DNS:${otherDNS}, DNS:${otherDNS}.cluster.local'")) -days 365

  for secret_name in eirini-webhooks-certs; do
    if kubectl -n "${EIRINI_NAMESPACE}" get secret "${secret_name}" >/dev/null 2>&1; then
      kubectl delete secret -n "${EIRINI_NAMESPACE}" "${secret_name}"
    fi

    echo "Creating the ${secret_name} secret in your kubernetes cluster..."
    kubectl create secret -n "${EIRINI_NAMESPACE}" generic "${secret_name}" --from-file=tls.crt=./tls.crt --from-file=tls.ca=./tls.crt --from-file=tls.key=./tls.key
  done

  echo "Done!"
}
popd
