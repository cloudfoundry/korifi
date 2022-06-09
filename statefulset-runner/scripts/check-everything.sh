#!/bin/bash

set -euo pipefail
IFS=$'\n\t'

RUN_DIR="$(cd "$(dirname "$0")" && pwd)"
EIRINI_CONTROLLER_DIR="$RUN_DIR/.."

if [ -z ${EIRINIUSER_PASSWORD+x} ]; then
  EIRINIUSER_PASSWORD="$(pass eirini/docker-hub)"
fi

export TELEPRESENCE_EXPOSE_PORT_START=10000
export TELEPRESENCE_SERVICE_NAME

clusterLock=$HOME/.kind-cluster.lock

ensure_kind_cluster() {
  local cluster_name
  cluster_name="$1"
  if ! kind get clusters | grep -q "$cluster_name"; then
    current_cluster="$(KUBECONFIG="$HOME/.kube/config" kubectl config current-context)" || true
    kindConfig=$(mktemp)
    cat <<EOF >>"$kindConfig"
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
EOF
    kind create cluster --name "$cluster_name" --config "$kindConfig" --wait 5m
    rm -f "$kindConfig"
    if [[ -n "$current_cluster" ]]; then
      KUBECONFIG="$HOME/.kube/config" kind export kubeconfig --name "$cluster_name"
      KUBECONFIG="$HOME/.kube/config" kubectl config use-context "$current_cluster"
    fi
  fi

  kind export kubeconfig --name "$cluster_name" --kubeconfig "$HOME/.kube/$cluster_name.yml"
}

run_unit_tests() {
  echo "Running unit tests"

  export GO111MODULE=on
  "$RUN_DIR"/run_unit_tests.sh "$@"
}

run_integration_tests() {
  echo "#########################################"
  echo "Running integration tests"
  echo "#########################################"
  echo

  "${EIRINI_CONTROLLER_DIR}/scripts/run_integration_tests.sh"
}

run_eats() {
  local cluster_name="eats"
  export KUBECONFIG="$HOME/.kube/$cluster_name.yml"
  ensure_kind_cluster "$cluster_name"

  echo "#########################################"
  echo "Running EATs against deployed eirini on $(kubectl config current-context)"
  echo "#########################################"
  echo

  cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  name: eirini-controller
EOF

  if [[ "$redeploy" == "true" ]]; then
    redeploy_prometheus
    redeploy_cert_manager
    redeploy_eirini_controller
  fi

  local service_name
  service_name=telepresence-$(uuidgen)

  local src_dir
  src_dir=$(mktemp -d)
  cp -a "$EIRINI_CONTROLLER_DIR" "$src_dir"
  cp "$KUBECONFIG" "$src_dir"
  trap "rm -rf $src_dir" EXIT

  export EIRINI_ADDRESS EIRINI_TLS_SECRET EIRINI_SYSTEM_NS EIRINI_WORKLOADS_NS INTEGRATION_KUBECONFIG TELEPRESENCE_SERVICE_NAME
  EIRINI_TLS_SECRET="eirini-webhook-certs"
  EIRINI_SYSTEM_NS="eirini-controller"
  EIRINI_WORKLOADS_NS="eirini-controller-workloads"
  INTEGRATION_KUBECONFIG="${src_dir}/$(basename "$KUBECONFIG")"
  TELEPRESENCE_EXPOSE_PORT_START=${service_name}

  KUBECONFIG="${INTEGRATION_KUBECONFIG}" telepresence --new-deployment "$service_name" \
    --method vpn-tcp \
    --run "${src_dir}/scripts/run_eats_tests.sh"
}

redeploy_prometheus() {
  kapp -y delete -a prometheus
  helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
  helm repo update
  helm -n eirini-controller template prometheus prometheus-community/prometheus | kapp -y deploy -a prometheus -f -
}

redeploy_cert_manager() {
  kapp -y delete -a cert-mgr
  kapp -y deploy -a cert-mgr -f https://github.com/cert-manager/cert-manager/releases/download/v1.8.0/cert-manager.yaml
}

redeploy_eirini_controller() {
  render_dir=$(mktemp -d)
  trap "rm -rf $render_dir" EXIT
  kbld -f "$EIRINI_CONTROLLER_DIR/deployment/scripts/assets/kbld-kind.yaml" \
    -f "$EIRINI_CONTROLLER_DIR/deployment/helm/values-template.yaml" \
    >"$EIRINI_CONTROLLER_DIR/deployment/helm/values.yaml"

  kind load docker-image --name $cluster_name eirini-controller:latest

  "$EIRINI_CONTROLLER_DIR/deployment/scripts/render-templates.sh" eirini-controller "$render_dir" \
    --values "$EIRINI_CONTROLLER_DIR/deployment/scripts/assets/value-overrides.yaml"
  for img in $(grep -oh "kbld:.*" "$EIRINI_CONTROLLER_DIR/deployment/helm/values.yaml"); do
    kind load docker-image --name eats "$img"
  done
  kapp -y delete -a eirini-controller
  kapp -y deploy -a eirini-controller -f "$render_dir/templates/"
}

run_linter() {
  echo "Running Linter"
  cd "$RUN_DIR"/.. || exit 1
  golangci-lint run
}

run_subset() {
  if [[ "$run_unit_tests" == "true" ]]; then
    run_unit_tests "$@"
  fi

  if [[ "$run_integration_tests" == "true" ]]; then
    run_integration_tests "$@"
  fi

  if [[ "$run_eats" == "true" ]]; then
    run_eats "$@"
  fi

  if [[ "$run_linter" == "true" ]]; then
    run_linter
  fi
}

RED=1
GREEN=2
print_message() {
  message=$1
  colour=$2
  printf "\\r\\033[00;3%sm[%s]\\033[0m\\n" "$colour" "$message"
}

run_everything() {
  print_message "about to run tests in parallel, it will be awesome" $GREEN
  print_message "ctrl-d panes when they are done" $RED
  local do_not_deploy="-n "
  if [[ "$redeploy" == "true" ]]; then
    do_not_deploy=""
  fi
  tmux new-window -n eirini-tests "/bin/bash -c \"$0 -u; bash --init-file <(echo 'history -s $0 -u')\""
  tmux split-window -h -p 50 "/bin/bash -c \"$0 -i $do_not_deploy; bash --init-file <(echo 'history -s $0 -i $do_not_deploy')\""
  tmux split-window -v -p 50 "/bin/bash -c \"$0 -e $do_not_deploy; bash --init-file <(echo 'history -s $0 -e $do_not_deploy')\""
  tmux select-pane -L
  tmux split-window -v -p 50 "/bin/bash -c \"$0 -l; bash --init-file <(echo 'history -s $0 -l')\""
}

main() {
  USAGE=$(
    cat <<EOF
Usage: check-everything.sh [options]
Options:
  -a  run all tests (default)
  -e  EATs tests
  -h  this help
  -i  integration tests
  -l  golangci-lint
  -n  do not redeploy eirini when running eats
  -u  unit tests
EOF
  )

  local run_eats="false" \
    run_unit_tests="false" \
    run_integration_tests="false" \
    run_linter="false" \
    redeploy="true" \
    run_subset="false"

  while getopts "auiefrnhl" opt; do
    case ${opt} in
      n)
        redeploy="false"
        ;;
      a)
        run_subset="false"
        ;;
      u)
        run_unit_tests="true"
        run_subset="true"
        ;;
      i)
        run_integration_tests="true"
        run_subset="true"
        ;;
      e)
        run_eats="true"
        run_subset="true"
        ;;
      l)
        run_linter="true"
        run_subset="true"
        ;;
      h)
        echo "$USAGE"
        exit 0
        ;;
      \?)
        echo "Invalid option: $OPTARG" 1>&2
        echo "$USAGE"
        exit 1
        ;;
      :)
        echo "Invalid option: $OPTARG requires an argument" 1>&2
        echo "$USAGE"
        exit 1
        ;;
    esac
  done
  shift $((OPTIND - 1))

  if [[ "$run_subset" == "true" ]]; then
    run_subset "$@"
  else
    run_everything
  fi
}

main "$@"
