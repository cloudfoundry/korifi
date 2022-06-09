#!/bin/bash

set -xeuo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
readonly EIRINI_DIR="$(readlink -f $SCRIPT_DIR/..)"
readonly CLUSTER_NAME="run-tests"
readonly TMP_DIR="$(mktemp -d)"
readonly EIRINI_BINS="$EIRINI_DIR/tmp"
readonly KIND_CONF="${TMP_DIR}/kind-config-run-tests"
readonly EIRINIUSER_PASSWORD=${EIRINIUSER_PASSWORD:-}
readonly TEST_SCRIPT="$@"

trap "rm -rf $TMP_DIR" EXIT

mkdir -p "$EIRINI_BINS"
trap "rm -rf $EIRINI_BINS" EXIT

main() {
  ensure_kind_cluster
  cleanup

  run_tests
}

cleanup() {
  kubectl --namespace eirini-test delete job,configmap,secret --all --wait=true

  for ns in $(kubectl get namespaces | grep "integration-test" | awk '{ print $1 }'); do
    echo Deleting leftover namespace $ns
    kubectl delete namespace --wait=false "$ns" || true
  done
}

ensure_kind_cluster() {
  if ! kind get clusters | grep -q "$CLUSTER_NAME"; then
    current_cluster="$(kubectl config current-context)" || true
    cat <<EOF >>"$KIND_CONF"
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraMounts:
  - containerPath: /eirini
    hostPath: $EIRINI_DIR
    readOnly: true
EOF
    kind create cluster --name "$CLUSTER_NAME" --config "$KIND_CONF" --wait 5m

    if [[ -n "$current_cluster" ]]; then
      kubectl config use-context "$current_cluster"
    fi
  fi
  kind export kubeconfig --name "$CLUSTER_NAME" --kubeconfig "$HOME/.kube/$CLUSTER_NAME.yml"
}

run_tests() {
  local pod_name

  kubectl apply -f "$SCRIPT_DIR/assets/kinda-run-tests/namespace.yml"
  kubectl apply -f "$SCRIPT_DIR/assets/kinda-run-tests/go-cache-pvc.yml"

  kubectl --namespace eirini-test create configmap test-config \
    --from-literal="TEST_SCRIPT=$TEST_SCRIPT"

  kubectl --namespace eirini-test create secret generic test-secret \
    --from-literal="EIRINIUSER_PASSWORD=${EIRINIUSER_PASSWORD}"

  kubectl apply -f "$SCRIPT_DIR/assets/kinda-run-tests/test-job.yml"

  for i in $(seq 120); do
    pod_name=$(kubectl --namespace eirini-test get pods -l "job-name=eirini-integration-tests" -o json | jq -r '.items[0].metadata.name')
    if [ "$pod_name" != "null" ]; then
      break
    fi
    sleep 1
  done

  if [ "$pod_name" == "null" ]; then
    echo "Test pod did not start!"
    exit 1
  fi

  kubectl --namespace eirini-test wait pod $pod_name --for=condition=Ready

  kubectl --namespace eirini-test logs -f job/eirini-integration-tests
}

main "$@"
