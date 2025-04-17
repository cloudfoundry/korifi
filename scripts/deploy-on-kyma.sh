#!/usr/bin/env bash

set -xeuo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT_DIR="${ROOT_DIR}/scripts"

WILDCARD_DOMAIN="$(kubectl get gateway.networking.istio.io -n kyma-system kyma-gateway -o jsonpath='{.spec.servers[0].hosts[0]}')"
CF_DOMAIN="${WILDCARD_DOMAIN:2}"

APPS_DOMAIN="apps.$CF_DOMAIN"
KORIFI_API="cfapi.$CF_DOMAIN"

# workaround for https://github.com/carvel-dev/kbld/issues/213
# kbld fails with git error messages in languages than other english
export LC_ALL=en_US.UTF-8

function validate_registry_params() {
  local registry_env_vars
  registry_env_vars="\$DOCKER_SERVER \$DOCKER_USERNAME \$DOCKER_PASSWORD \$REPOSITORY_PREFIX \$KPACK_BUILDER_REPOSITORY"

  if [ -z ${DOCKER_SERVER+x} ] &&
    [ -z ${DOCKER_USERNAME+x} ] &&
    [ -z ${DOCKER_PASSWORD+x} ] &&
    [ -z ${REPOSITORY_PREFIX+x} ] &&
    [ -z ${KPACK_BUILDER_REPOSITORY+x} ]; then

    echo "None of $registry_env_vars are set. Assuming local registry."
    DOCKER_SERVER="$LOCAL_DOCKER_REGISTRY_ADDRESS"
    DOCKER_USERNAME="user"
    DOCKER_PASSWORD="password"
    REPOSITORY_PREFIX="$DOCKER_SERVER/"
    KPACK_BUILDER_REPOSITORY="$DOCKER_SERVER/kpack-builder"

    return
  fi

  echo "The following env vars should either be set together or none of them should be set: $registry_env_vars"
  echo "$DOCKER_SERVER $DOCKER_USERNAME $DOCKER_PASSWORD $REPOSITORY_PREFIX $KPACK_BUILDER_REPOSITORY" >/dev/null
}

function install_dependencies() {
  pushd "${ROOT_DIR}" >/dev/null
  {
    "${SCRIPT_DIR}/install-dependencies.sh" -i
  }
  popd >/dev/null
}

function ca_from_secret() {
  local secret_name="$1"

  kubectl get secret "$secret_name" -n korifi -o jsonpath='{.data.ca\.crt}'
}

function deploy_korifi() {
  pushd "${ROOT_DIR}" >/dev/null
  {

    echo "Building korifi values file..."

    make generate manifests

    docker_server=$(kubectl -n kyma-system get secret dockerregistry-config-external -o jsonpath='{.data.pushRegAddr}' | base64 -d)
    version=$(uuidgen)
    for component in api controllers job-task-runner kpack-image-builder statefulset-runner; do
      docker build -f $component/Dockerfile -t $docker_server/korifi/$component:$version .
      docker push $docker_server/korifi/$component:$version
    done

    echo "Deploying korifi..."
    helm dependency update helm/korifi

    helm upgrade --install korifi helm/korifi \
      --namespace korifi \
      --set=adminUserName="cf-admin" \
      --set=defaultAppDomainName="$APPS_DOMAIN" \
      --set=certManager.enabled="false" \
      --set=logLevel="debug" \
      --set=stagingRequirements.buildCacheMB="1024" \
      --set=api.apiServer.url="$KORIFI_API" \
      --set=controllers.taskTTL="5s" \
      --set=jobTaskRunner.jobTTL="5s" \
      --set=containerRepositoryPrefix="$docker_server/kpack-builder" \
      --set=kpackImageBuilder.clusterBuilderName="kind-builder" \
      --set=networking.gatewayClass="isito" \
      --set=networking.gatewayPorts.http="32080" \
      --set=networking.gatewayPorts.https="32443" \
      --set=experimental.managedServices.enabled="true" \
      --set=experimental.securityGroups.enabled="true" \
      --set=experimental.managedServices.trustInsecureBrokers="true" \
      --set="systemImagePullSecrets={dockerregistry-config-external}" \
      --set=api.image="$docker_server/korifi/api:$version" \
      --set=controllers.image="$docker_server/korifi/controllers:$version" \
      --set=kpackImageBuilder.image="$docker_server/korifi/kpack-image-builder:$version" \
      --set=statefulsetRunner.image="$docker_server/korifi/statefulset-runner:$version" \
      --set=jobTaskRunner.image="$docker_server/korifi/job-task-runner:$version" \
      --wait
  }
  popd >/dev/null
}

function create_namespaces() {
  local security_policy

  security_policy="restricted"

  for ns in cf korifi; do
    cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Namespace
metadata:
  labels:
    pod-security.kubernetes.io/audit: $security_policy
    pod-security.kubernetes.io/enforce: $security_policy
  name: $ns
EOF
  done
}

function create_registry_secret() {
  local docker_server=$(kubectl -n kyma-system get secret dockerregistry-config-external -o jsonpath='{.data.pushRegAddr}' | base64 -d)
  local user_name=$(kubectl -n kyma-system get secret dockerregistry-config-external -o jsonpath='{.data.username}' | base64 -d)
  local password=$(kubectl -n kyma-system get secret dockerregistry-config-external -o jsonpath='{.data.password}' | base64 -d)

  kubectl delete -n cf secret image-registry-credentials --ignore-not-found
  kubectl create secret -n cf docker-registry image-registry-credentials \
    --docker-server="$docker_server" \
    --docker-username="$user_name" \
    --docker-password="$password"
}

function create_cluster_builder() {
  (
    export BUILDER_TAG="$KPACK_BUILDER_REPOSITORY"
    envsubst <"$SCRIPT_DIR/assets/templates/kind-builder.yml" | kubectl apply -f -
  )
  kubectl wait --for=condition=ready clusterbuilder --all=true --timeout=15m
}

function configure_istio_module() {
  echo "************************************************"
  echo " Selecting the experimental channel for istio kyma module "
  echo "************************************************"

  kyma alpha module add istio -c experimental

  echo "************************************************"
  echo " Enabling Gateway API on the kyma module "
  echo "************************************************"
  kubectl -n kyma-system patch istios.operator.kyma-project.io default --type merge --patch-file $SCRIPT_DIR/assets/kyma-istio-patch.yaml

  echo "************************************************"
  echo " Checking Kyma Module Readiness "
  echo "************************************************"
  kubectl -n kyma-system wait --for=jsonpath='{.status.state}'=Ready --timeout=120s istios.operator.kyma-project.io/default
}

function install_docker_registry_module() {
  echo "************************************************"
  echo " Intalling the Docker Registry Kyma Module "
  echo "************************************************"
  kubectl apply -f https://github.com/kyma-project/docker-registry/releases/latest/download/dockerregistry-operator.yaml
  kubectl apply -n kyma-system -f - <<EOF
apiVersion: operator.kyma-project.io/v1alpha1
kind: DockerRegistry
metadata:
  name: default
  namespace: kyma-system
spec:
  externalAccess:
    enabled: true
EOF

  echo "************************************************"
  echo " Checking Docker Registry Readiness "
  echo "************************************************"
  kubectl wait --for=jsonpath='.status.state'=Ready -n kyma-system dockerregistries.operator.kyma-project.io default --timeout=5m

  echo "************************************************"
  echo " Logging in to Docker Registry "
  echo "************************************************"
  local docker_server=$(kubectl -n kyma-system get secret dockerregistry-config-external -o jsonpath='{.data.pushRegAddr}' | base64 -d)
  local user_name=$(kubectl -n kyma-system get secret dockerregistry-config-external -o jsonpath='{.data.username}' | base64 -d)
  local password=$(kubectl -n kyma-system get secret dockerregistry-config-external -o jsonpath='{.data.password}' | base64 -d)

  docker login -u "$user_name" -p "$password" "$docker_server"
}

function create_korifi_certificates() {
  kubectl apply -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: korifi
---
apiVersion: cert.gardener.cloud/v1alpha1
kind: Certificate
metadata:
  name: korifi-api-ingress-cert
  namespace: korifi
spec:
  commonName: $CF_DOMAIN
  dnsNames:
  - "$KORIFI_API"
  secretRef:
    name: korifi-api-ingress-cert
    namespace: korifi
---
apiVersion: cert.gardener.cloud/v1alpha1
kind: Certificate
metadata:
  name: korifi-workloads-ingress-cert
  namespace: korifi
spec:
  commonName: $CF_DOMAIN
  dnsNames:
  - "*.$APPS_DOMAIN"
  secretRef:
    name: korifi-workloads-ingress-cert
    namespace: korifi
---
apiVersion: cert.gardener.cloud/v1alpha1
kind: Certificate
metadata:
  name: korifi-controllers-webhook-cert
  namespace: korifi
spec:
  commonName: korifi-controllers-webhook-service.korifi
  dnsNames:
  - korifi-controllers-webhook-service.korifi.svc
  - korifi-controllers-webhook-service.korifi.svc.cluster.local
  secretRef:
    name: korifi-controllers-webhook-cert
    namespace: korifi
  isCA: true
  issuerRef:
    name: kim-snatch-kyma
    namespace: kyma-system
---
apiVersion: cert.gardener.cloud/v1alpha1
kind: Certificate
metadata:
  name: korifi-kpack-image-builder-webhook-cert
  namespace: korifi
spec:
  commonName: korifi-kpack-image-builder-webhook-service.korifi
  dnsNames:
  - korifi-kpack-image-builder-webhook-service.korifi.svc
  - korifi-kpack-image-builder-webhook-service.korifi.svc.cluster.local
  secretRef:
    name: korifi-kpack-image-builder-webhook-cert
    namespace: korifi
  isCA: true
  issuerRef:
    name: kim-snatch-kyma
    namespace: kyma-system
---
apiVersion: cert.gardener.cloud/v1alpha1
kind: Certificate
metadata:
  name: korifi-statefulset-runner-webhook-cert
  namespace: korifi
spec:
  commonName: korifi-statefulset-runner-webhook-service.korifi
  dnsNames:
  - korifi-statefulset-runner-webhook-service.korifi.svc
  - korifi-statefulset-runner-webhook-service.korifi.svc.cluster.local
  secretRef:
    name: korifi-statefulset-runner-webhook-cert
    namespace: korifi
  isCA: true
  issuerRef:
    name: kim-snatch-kyma
    namespace: kyma-system
EOF

  echo "Waiting for korifi certificates..."
  retry kubectl get --namespace korifi secret korifi-api-ingress-cert
  retry kubectl get --namespace korifi secret korifi-workloads-ingress-cert
  retry kubectl get --namespace korifi secret korifi-controllers-webhook-cert
  retry kubectl get --namespace korifi secret korifi-kpack-image-builder-webhook-cert
  retry kubectl get --namespace korifi secret korifi-statefulset-runner-webhook-cert
}

set_dns_entries() {
  # TODO: untested
  local ingress_host
  ingress_host="$(kubectl get svc -n korifi-gateway korifi-istio -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')"
  if [ -z "$ingress_host" ]; then
    ingress_host="$(kubectl get svc -n korifi-gateway korifi-istio -o jsonpath='{.status.loadBalancer.ingress[0].ip}')"
  fi

  kubectl apply -f - <<EOF
---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
    # Let Gardener manage this DNS entry.
    dns.gardener.cloud/class: garden
  name: cf-api-ingress
  namespace: korifi
spec:
  dnsName: $KORIFI_API
  ttl: 600
  targets:
  - $ingress_host

---
apiVersion: dns.gardener.cloud/v1alpha1
kind: DNSEntry
metadata:
  annotations:
    # Let Gardener manage this DNS entry.
    dns.gardener.cloud/class: garden
  name: cf-apps-ingress
  namespace: korifi
spec:
  dnsName: "*.$APPS_DOMAIN"
  ttl: 600
  targets:
  - $ingress_host
EOF

  kubectl wait --for=jsonpath='.status.state'=Ready -n korifi dnsentry cf-api-ingress --timeout=5m
  kubectl wait --for=jsonpath='.status.state'=Ready -n korifi dnsentry cf-apps-ingress --timeout=5m

}

retry() {
  until $@; do
    echo -n .
    sleep 1
  done
  echo
}

function main() {
  make -C "$ROOT_DIR" bin/yq

  install_dependencies
  configure_istio_module
  install_docker_registry_module

  create_namespaces
  create_registry_secret
  create_korifi_certificates
  deploy_korifi
  exit 0
  set_dns_entries
  create_cluster_builder
}

main "$@"
