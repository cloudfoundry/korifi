#!/usr/bin/env bash

echo "Will now generate tls.ca tls.crt and tls.key files"

keys="$(mktemp -d)"
trap 'rm -rf "${keys}"' EXIT

readonly CONTROLLERS_NAMESPACE=cf-k8s-controllers-system
otherDNS="$1"

pushd "${keys}"
{
  kubectl create namespace "${CONTROLLERS_NAMESPACE}" || true

  openssl req -x509 -newkey rsa:4096 -keyout tls.key -out tls.crt -nodes -subj '/CN=localhost' -addext "subjectAltName = DNS:${otherDNS}" -days 365

  for secret_name in cf-k8s-workloads-ingress-cert; do
    if kubectl -n "${CONTROLLERS_NAMESPACE}" get secret "${secret_name}" >/dev/null 2>&1; then
      kubectl delete secret -n "${CONTROLLERS_NAMESPACE}" "${secret_name}"
    fi

    echo "Creating the ${secret_name} secret in your kubernetes cluster..."
    kubectl create secret -n "${CONTROLLERS_NAMESPACE}" generic "${secret_name}" --from-file=tls.crt=./tls.crt --from-file=tls.ca=./tls.crt --from-file=tls.key=./tls.key
  done

  echo "Done!"
}
popd
