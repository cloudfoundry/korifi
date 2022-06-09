#!/bin/bash

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

source "$SCRIPT_DIR/assets/cf4k8s/cc-commons.sh"

build_ccng_image
publish-image
patch-cf-api-component cf-api-server
patch-cf-api-component cf-api-clock
patch-cf-api-component cf-api-worker
patch-cf-api-component cf-api-deployment-updater
