#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT_DIR="${ROOT_DIR}/scripts"
API_DIR="${ROOT_DIR}/api"
CONTROLLER_DIR="${ROOT_DIR}/controllers"
export PATH="${PATH}:${API_DIR}/bin"

OPENSSL_VERSION="$(openssl version | awk '{ print $1 }')"

source "$SCRIPT_DIR/common.sh"

function usage_text() {
  cat <<EOF
Usage:
  $(basename "$0") <kind cluster name>

flags:
  -v, --verbose
      Verbose output (bash -x).

  -c, --controllers-only
      Skips all steps except for building and installing
      controllers. (This will fail unless the script is
      being re-run.)

  -a, --api-only
      Skips all steps except for building and installing
      the API shim. (This will fail unless the script is
      being re-run.)
EOF
  exit 1
}

cluster=""
use_local_registry=""
controllers_only=""
api_only=""
controllers_debug=""
while [[ $# -gt 0 ]]; do
  i=$1
  case $i in
  -c | --controllers-only)
    controllers_only="true"
    shift
    ;;
  -a | --api-only)
    api_only="true"
    shift
    ;;
  -v | --verbose)
    set -x
    shift
    ;;
  -h | --help | help)
    usage_text >&2
    exit 0
    ;;
  esac
done

function create_tls_secret() {
  local secret_name=${1:?}
  local secret_namespace=${2:?}
  local tls_cn=${3:?}

  tmp_dir=$(mktemp -d -t cf-tls-XXXXXX)

  if [[ "${OPENSSL_VERSION}" == "OpenSSL" ]]; then
    openssl req -x509 -newkey rsa:4096 \
      -keyout ${tmp_dir}/tls.key \
      -out ${tmp_dir}/tls.crt \
      -nodes \
      -subj "/CN=${tls_cn}" \
      -addext "subjectAltName = DNS:${tls_cn}" \
      -days 365
  else
    openssl req -x509 -newkey rsa:4096 \
      -keyout ${tmp_dir}/tls.key \
      -out ${tmp_dir}/tls.crt \
      -nodes \
      -subj "/CN=${tls_cn}" \
      -extensions SAN -config <(cat /etc/ssl/openssl.cnf <(printf "[ SAN ]\nsubjectAltName='DNS:${tls_cn}'")) \
      -days 365
  fi

  cat <<EOF >${tmp_dir}/kustomization.yml
secretGenerator:
- name: ${secret_name}
  namespace: ${secret_namespace}
  files:
  - tls.crt=tls.crt
  - tls.key=tls.key
  type: "kubernetes.io/tls"
generatorOptions:
  disableNameSuffixHash: true
EOF

  kubectl apply -k $tmp_dir

  rm -r ${tmp_dir}
}

# undo *_IMG changes in config and reference
function clean_up_img_refs() {
  cd "${ROOT_DIR}"
  unset IMG_CONTROLLERS
  unset IMG_API
  make build-reference
}
trap clean_up_img_refs EXIT

function install_dependencies() {
  if [[ -n "${controllers_only}" ]]; then return 0; fi
  if [[ -n "${api_only}" ]]; then return 0; fi

  pushd "${ROOT_DIR}" >/dev/null
  {
    ${SCRIPT_DIR}/install-dependencies.sh -g gcr.json
  }
  popd >/dev/null
}

function deploy_cf_k8s_controllers() {
  if [[ -n "${api_only}" ]]; then return 0; fi

  APP_FQDN="pinniped-poc-apps.cf-for-k8s.relint.rocks"
  pushd "${ROOT_DIR}" >/dev/null
  {
    export KUBEBUILDER_ASSETS="${ROOT_DIR}/testbin/bin"
    echo "${PWD}"
    make generate-controllers
    IMG_CONTROLLERS=${IMG_CONTROLLERS:-"gcr.io/cf-relint-greengrass/pinniped-spike/controllers:$(uuidgen)"}
    export IMG_CONTROLLERS
    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
        make docker-build-controllers
        make docker-push-controllers
    fi

    make install-crds
    make deploy-controllers

    create_tls_secret "cf-k8s-workloads-ingress-cert" "cf-k8s-controllers-system" "*.${APP_FQDN}"
  }
  popd >/dev/null

  kubectl rollout status deployment/cf-k8s-controllers-controller-manager -w -n cf-k8s-controllers-system

  sed 's/vcap\.me/'${APP_FQDN}'/' ${CONTROLLER_DIR}/config/samples/cfdomain.yaml | kubectl apply -f-
}

function deploy_cf_k8s_api() {
  if [[ -n "${controllers_only}" ]]; then return 0; fi

  API_FQDN="pinniped-poc-api.cf-for-k8s.relint.rocks"

  pushd "${ROOT_DIR}" >/dev/null
  {
    IMG_API=${IMG_API:-"gcr.io/cf-relint-greengrass/pinniped-spike/api:$(uuidgen)"}
    export IMG_API
    if [[ -z "${SKIP_DOCKER_BUILD:-}" ]]; then
      make docker-build-api
      make docker-push-api
    fi

    make deploy-api

    create_tls_secret "cf-k8s-api-ingress-cert" "cf-k8s-api-system" "${API_FQDN}"
  }
  popd >/dev/null
}

install_dependencies
deploy_cf_k8s_controllers
deploy_cf_k8s_api
