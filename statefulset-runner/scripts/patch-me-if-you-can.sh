#!/bin/bash

set -euo pipefail

IFS=$'\n\t'

USAGE=$(
  cat <<EOF
Usage: patch-me-if-you-can.sh [options] [ <component-name> ... ]
Options:
  -c <cluster-name>  - required unless skipping deloyment
  -s  skip docker builds
  -S  skip deployment (only update the eirini release SHAs)
  -o <additional-values.yml>  - use additional values from file
  -h  this help
EOF
)
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
readonly EIRINI_BASEDIR=$(realpath "$SCRIPT_DIR/..")
readonly EIRINI_RELEASE_BASEDIR=$(realpath "$SCRIPT_DIR/../../eirini-release")
readonly EIRINI_PRIVATE_CONFIG_BASEDIR=$(realpath "$SCRIPT_DIR/../../eirini-private-config")
readonly EIRINI_CI_BASEDIR="$HOME/workspace/eirini-ci"
readonly CF4K8S_DIR="$HOME/workspace/cf-for-k8s"
readonly CAPI_DIR="$HOME/workspace/capi-release"
readonly CAPIK8S_DIR="$HOME/workspace/capi-k8s-release"
readonly PATCH_TAG='patch-me-if-you-can'

main() {
  if [ "$#" == "0" ]; then
    echo "$USAGE"
    exit 1
  fi

  local cluster_name="" additional_values skip_docker_build="false" skip_deployment="false"

  additional_values=""
  while getopts "hc:o:sS" opt; do
    case ${opt} in
      h)
        echo "$USAGE"
        exit 0
        ;;
      c)
        cluster_name=$OPTARG
        ;;
      s)
        skip_docker_build="true"
        ;;
      S)
        skip_deployment="true"
        ;;
      o)
        additional_values=$OPTARG
        if ! [[ -f $additional_values ]]; then
          echo "Provided values file does not exist: $additional_values"
          echo $USAGE
          exit 1
        fi
        ;;
      \?)
        echo "Invalid option: $OPTARG" 1>&2
        echo "$USAGE"
        ;;
      :)
        echo "Invalid option: $OPTARG requires an argument" 1>&2
        echo "$USAGE"
        ;;
    esac
  done
  shift $((OPTIND - 1))

  if [[ "$skip_deployment" == "false" && -z "$cluster_name" ]]; then
    echo "Cluster name not provided"
    echo "$USAGE"
    exit 1
  fi

  if [[ "$(current_cluster_name)" =~ "gke_.*${cluster_name}\$" ]]; then
    echo "Your current cluster is $(current_cluster_name), but you want to update $cluster_name. Please target $cluster_name"
    echo "gcloudcluster $cluster_name"
    exit 1
  fi

  echo "Checking out latest stable cf-for-k8s..."
  checkout_cf4k8s_develop

  if [ "$skip_docker_build" != "true" ]; then
    if [ "$#" == "0" ]; then
      echo "No components specified. Nothing to do."
      echo "If you want to helm upgrade without building containers, please pass the '-s' flag"
      exit 0
    fi
    local component
    for component in "$@"; do
      if is_cloud_controller $component; then
        checkout_stable_cf_for_k8s_deps
        build_ccng_image
      else
        update_component "$component"
      fi
    done
  fi

  if [[ "$skip_deployment" == "true" ]]; then
    exit 0
  fi

  pull_private_config
  patch_cf_for_k8s "$additional_values"
  deploy_cf "$cluster_name"
}

is_cloud_controller() {
  local component
  component="$1"
  [[ "$component" =~ cloud.controller ]] || [[ "$component" =~ "ccng" ]] || [[ "$component" =~ "capi" ]] || [[ "$component" =~ "cc" ]]
}

checkout_cf4k8s_develop() {
  pushd "$CF4K8S_DIR"
  {
    echo "Cleaning dirty state in cf-for-k8s..."
    git checkout . && git clean -ffd
    echo "Checking out pending-prs branch"
    git checkout develop
    git pull ef develop
  }
  popd
}

checkout_stable_cf_for_k8s_deps() {
  local capi_k8s_release_sha ccng_image ccng_sha

  echo "Dear future us, take a deep breath as I am checking out stable revisions of cf-for-k8s dependencies for you..."

  echo "Getting the vendored revision of capi-k8s-release..."

  pushd "$CF4K8S_DIR"
  {
    capi_k8s_release_sha=$(yq eval '.directories.[] | select(.path == "config/capi/_ytt_lib/capi-k8s-release").contents[0].git.sha' vendir.lock.yml)
    echo "capi-k8s-release version: $capi_k8s_release_sha"
  }
  popd

  pushd "$CAPIK8S_DIR"
  {
    ccng_image="$(git show "$capi_k8s_release_sha:values/images.yml" | awk '/ccng:/ { print $2 }')"
    echo "Pulling the ccng image in order to read its metadata"
    docker pull $ccng_image
    ccng_sha="$(docker inspect "$ccng_image" | jq -r '.[] | .Config.Labels["io.deplab.metadata"]' | jq -r '.dependencies[] | select(.source.type=="git") | .source.version.commit')"
    echo "cloud_controller_ng version: $ccng_sha"
  }
  popd

  pushd "$CAPI_DIR/src/cloud_controller_ng"
  {
    git stash
    git checkout "$ccng_sha"
    git stash pop
  }
  popd

  echo "All done!"
}

build_ccng_image() {
  export IMAGE_DESTINATION_CCNG="docker.io/eirini/dev-ccng"
  export IMAGE_DESTINATION_CF_API_CONTROLLERS="docker.io/eirini/dev-controllers"
  export IMAGE_DESTINATION_REGISTRY_BUDDY="docker.io/eirini/dev-registry-buddy"
  export IMAGE_DESTINATION_BACKUP_METADATA="docker.io/eirini/dev-backup-metadata"
  git -C "$CAPIK8S_DIR" checkout values/images.yml
  "$CAPIK8S_DIR"/scripts/build-into-values.sh "$CAPIK8S_DIR/values/images.yml"
  "$CAPIK8S_DIR"/scripts/bump-cf-for-k8s.sh
}

update_component() {
  local component
  component=$1

  echo "--- Patching component $component ---"
  docker_build "$component"
  docker_push "$component"
  update_image_in_yaml_files "$component"
}

docker_build() {
  echo "Building docker image for $1"
  pushd "$EIRINI_BASEDIR"
  make --directory=docker "$component" TAG="$PATCH_TAG"
  popd
}

docker_push() {
  echo "Pushing docker image for $1"
  pushd "$EIRINI_BASEDIR"
  make --directory=docker push-$component TAG="$PATCH_TAG"
  popd
}

update_image_in_yaml_files() {
  local image_name new_image_ref
  image_name="$1"
  new_image_ref="$(docker inspect --format='{{index .RepoDigests 0}}' "eirini/${1}:$PATCH_TAG")"
  echo "Updating $image_name to ref $new_image_ref"
  sed -i -e "s|eirini/${image_name}@sha256:.*$|${new_image_ref}|g" "$EIRINI_RELEASE_BASEDIR/helm/values.yaml"
}

patch_cf_for_k8s() {
  local render_dir
  render_dir="$(mktemp -d)"
  trap "rm -rf $render_dir" EXIT

  "$EIRINI_RELEASE_BASEDIR/scripts/render-templates.sh" cf-system "$render_dir" --values "$EIRINI_RELEASE_BASEDIR/scripts/assets/cf-for-k8s-value-overrides.yml"

  rm -rf "$CF4K8S_DIR/build/eirini/_vendir/eirini"
  mv "${render_dir}/templates" "$CF4K8S_DIR/build/eirini/_vendir/eirini"

  "$CF4K8S_DIR"/build/eirini/build.sh
}

deploy_cf() {
  local cluster_name
  cluster_name="$1"
  shift 1
  kapp deploy -a cf -f <(
    ytt -f "$CF4K8S_DIR/config" \
      -f "$EIRINI_CI_BASEDIR/cf-for-k8s" \
      -f "$EIRINI_PRIVATE_CONFIG_BASEDIR/environments/kube-clusters/"${cluster_name}"/default-values.yml" \
      -f "$EIRINI_PRIVATE_CONFIG_BASEDIR/environments/kube-clusters/"${cluster_name}"/loadbalancer-values.yml" \
      $@
  ) -y
}

pull_private_config() {
  pushd "$EIRINI_PRIVATE_CONFIG_BASEDIR"
  git pull --rebase
  popd
}

current_cluster_name() {
  kubectl config current-context | cut -d / -f 1
}

main "$@"
