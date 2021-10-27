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

retry() {
  until $@; do
    echo -n .
    sleep 1
  done
  echo
}

# Hierarchical namespace controller is quite asynchronous. There is no
# guarantee that the operations below would succeed on first invocation,
# so retry until they do.
echo -n waiting for hns controller to be ready and servicing validating webhooks
retry kubectl create namespace ping-hnc
retry kubectl hns create -n ping-hnc ping-hnc-child
retry kubectl get namespace ping-hnc-child
retry kubectl hns set --allowCascadingDeletion ping-hnc
retry kubectl delete namespace ping-hnc --wait=false

# The eirini controller requires a service account and rolebinding, which are
# used by the statefulset controller to be able to create pods
retry kubectl hns config set-resource serviceaccounts --mode Propagate
