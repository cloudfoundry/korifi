#!/usr/bin/env bash

set -euo pipefail

VERSION=${1:-}
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPT_DIR="${ROOT_DIR}/scripts"
RELEASE_DIR="$ROOT_DIR/release-$VERSION"

mkdir -p $RELEASE_DIR
cp $ROOT_DIR/CHANGELOG.md $RELEASE_DIR/
cp $SCRIPT_DIR/assets/korifi-kbld-release.yml $RELEASE_DIR/

yq -i "with(.destinations[]; .tags=[\"latest\", \"$VERSION\"])" "$RELEASE_DIR/korifi-kbld-release.yml"
yq -i "with(.destinations[]; .newImage |= sub(\"registry\",\"$DOCKER_REGISTRY\"))" "$RELEASE_DIR/korifi-kbld-release.yml"
yq -i "with(.destinations[]; .newImage |= sub(\"vrelease\",\"$VERSION\"))" "$RELEASE_DIR/korifi-kbld-release.yml"

kbld \
      -f "$RELEASE_DIR/korifi-kbld-release.yml" \
      -f "helm/korifi/values.yaml" \
      --images-annotation=false >$RELEASE_DIR/values.yaml


tar -czf $RELEASE_DIR/korifi-helm.tar.gz -C helm .