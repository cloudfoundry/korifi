#!/bin/bash

set -euxo pipefail

readonly HNC_VERSION="v0.8.0"
readonly HNC_PLATFORM="$(go env GOHOSTOS)_$(go env GOHOSTARCH)"
readonly HNC_BIN="$PWD/bin"
export PATH="$HNC_BIN:$PATH"

mkdir -p "$HNC_BIN"
curl -L "https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/kubectl-hns_${HNC_PLATFORM}" -o "${HNC_BIN}/kubectl-hns"
chmod +x "${HNC_BIN}/kubectl-hns"

kubectl label ns kube-system hnc.x-k8s.io/excluded-namespace=true --overwrite
kubectl label ns kube-public hnc.x-k8s.io/excluded-namespace=true --overwrite
kubectl label ns kube-node-lease hnc.x-k8s.io/excluded-namespace=true --overwrite
kubectl apply -f "https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/hnc-manager.yaml"
kubectl rollout status deployment/hnc-controller-manager -w -n hnc-system

# Hierarchical namespace controller is quite asynchronous. There is no
# guarantee that the operations below would succeed on first invocation,
# so retry until they do.
echo -n waiting for hns controller to be ready and servicing validating webhooks
until kubectl create namespace ping-hnc; do
  echo -n .
  sleep 0.5
done
until kubectl hns create -n ping-hnc ping-hnc-child; do
  echo -n .
  sleep 0.5
done
until kubectl get namespace ping-hnc-child; do
  echo -n .
  sleep 0.5
done
until kubectl hns set --allowCascadingDeletion ping-hnc; do
  echo -n .
  sleep 0.5
done
until kubectl delete namespace ping-hnc --wait=false; do
  echo -n .
  sleep 0.5
done
echo
