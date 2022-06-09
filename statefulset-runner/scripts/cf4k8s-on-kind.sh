#!/bin/bash

set -euo pipefail

USAGE=$(
  cat <<EOF
Usage: cf4k8s-on-kind.sh [options]
Options:
  -c  use local cloud_controller_ng
  -e  use local eirini
  -d  recreate cluster with new values file
  -h  this help
EOF
)

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

CLUSTER_NAME=${CLUSTER_NAME:-cf-for-k8s}
CF_DOMAIN=${CF_DOMAIN:-vcap.me}
EIRINI_RELEASE_BASEDIR=${EIRINI_RELEASE_BASEDIR:-$HOME/workspace/eirini-release}
EIRINI_CI_BASEDIR=${EIRINI_CI_BASEDIR:-$HOME/workspace/eirini-ci}
CF4K8S_DIR="$HOME/workspace/cf-for-k8s"
VALUES_FILE="$SCRIPT_DIR/values/$CLUSTER_NAME.cf-values.yml"

source "$SCRIPT_DIR/assets/cf4k8s/cc-commons.sh"

main() {
  use_local_cc="false"
  use_local_eirini="false"
  delete_environment="false"
  while getopts "cdeh" opt; do
    case ${opt} in
      h)
        echo "$USAGE"
        exit 0
        ;;
      c)
        use_local_cc="true"
        ;;
      d)
        delete_environment="true"
        ;;
      e)
        use_local_eirini="true"
        ;;
    esac
  done
  shift $((OPTIND - 1))

  if [[ ! -f "$VALUES_FILE" ]] || [[ "$delete_environment" == "true" ]]; then
    rm -f "$VALUES_FILE"
    # ask early for pass passphrase if required
    docker_username=eiriniuser
    docker_password=$(pass eirini/docker-hub)
    docker_repo_prefix=eirini
  else
    echo " ⚠️  WARNING: Using existing values file: $VALUES_FILE! If you want a clean deployment consider deleting this file!"
  fi

  ensure-kind-cluster
  generate-values-file
  build-eirini
  build-cc
  deploy
}

ensure-kind-cluster() {
  if [[ "$delete_environment" == "true" ]]; then
    echo "🗑  Deleting kind cluster"

    kind delete cluster --name cf-for-k8s
  fi

  if ! kind get clusters | grep -q "$CLUSTER_NAME"; then
    # don't install the default CNI as it doesn't support NetworkPolicies
    # use the cluster config in assets until cf-for-k8s release > 2.1.1
    # which will contain the containerd configuration
    kind create cluster \
      --config <(yq e '.networking.disableDefaultCNI = true' $SCRIPT_DIR/assets/kind/cluster.yml) \
      --image kindest/node:v1.19.1 \
      --name "$CLUSTER_NAME"
  else
    echo "⚠️  Using existing kind cluster"
    kind export kubeconfig --name $CLUSTER_NAME
  fi
}

generate-values-file() {
  if [[ ! -f "$VALUES_FILE" ]]; then
    echo "🌱 Generating new values file"

    mkdir -p "$(dirname $VALUES_FILE)"

    pushd "$CF4K8S_DIR"
    {
      ./hack/generate-values.sh -d "$CF_DOMAIN" >"$VALUES_FILE"

      cat <<EOF >>"$VALUES_FILE"
app_registry:
  hostname: https://index.docker.io/v1/
  repository_prefix: "$docker_repo_prefix"
  username: "$docker_username"
  password: "$docker_password"

add_metrics_server_components: true
enable_automount_service_account_token: true
load_balancer:
  enable: false
metrics_server_prefer_internal_kubelet_address: true
remove_resource_requirements: true
use_first_party_jwt_tokens: true
EOF
    }
    popd

  fi
}

build-eirini() {
  if [[ "$use_local_eirini" == "true" ]]; then
    EIRINI_RENDER_DIR=$(mktemp -d)
    trap "rm -rf $EIRINI_RENDER_DIR" EXIT

    echo "🔨 Building local eirini yamls"
    "$EIRINI_RELEASE_BASEDIR/scripts/render-templates.sh" cf-system "$EIRINI_RENDER_DIR"

    echo "🏍  Vendoring local templates"
    mv "$EIRINI_RENDER_DIR/templates" "$EIRINI_RENDER_DIR/eirini"
    pushd "$CF4K8S_DIR"
    {
      vendir sync -d "build/eirini/_vendir=$EIRINI_RENDER_DIR"
    }
    popd

    echo "🏞  Rendering eirini with ytt and kbld"
    ytt --ignore-unknown-comments \
      -f "$CF4K8S_DIR/build/eirini/_vendir/eirini/core" \
      -f "$CF4K8S_DIR/build/eirini/_vendir/eirini/events" \
      -f "$CF4K8S_DIR/build/eirini/_vendir/eirini/workloads" \
      -f "$CF4K8S_DIR/build/eirini/overlays" |
      kbld -f - -f "$SCRIPT_DIR/kbld-local-eirini.yml" \
        >"$CF4K8S_DIR/config/eirini/_ytt_lib/eirini/rendered.yml"

    echo "📦 Loading our images on kind"
    for img in $(grep -oh "kbld:.*" "$CF4K8S_DIR/config/eirini/_ytt_lib/eirini/rendered.yml"); do
      kind load docker-image --name cf-for-k8s "$img"
    done
  else
    pushd "$CF4K8S_DIR"
    {
      echo "🏞  Rendering eirini with ytt and kbld"
      ./build/eirini/build.sh
    }
    popd
  fi
}

build-cc() {
  if [[ "$use_local_cc" == "true" ]]; then
    echo "🔨 Building local cloud_controller_ng"
    # build & bump cc
    build_ccng_image
    push-to-docker
    sed -i "s|ccng: .*$|ccng: $CCNG_IMAGE:$TAG|" "$HOME/workspace/capi-k8s-release/values/images.yml"
    "$HOME/workspace/capi-k8s-release/scripts/bump-cf-for-k8s.sh"
  fi
}

deploy() {
  echo "⚙️  Deploying Calico"
  # install Calico to get NetworkPolicy support
  kapp deploy -y -a calico -f https://docs.projectcalico.org/manifests/calico.yaml

  echo "⚙️  Deploying cf-for-k8s"
  # deploy everything
  kapp deploy -y -a cf -f <(ytt -f "$HOME/workspace/cf-for-k8s/config" -f "$VALUES_FILE" $@)
}

main "$@"
