#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

export SMOKE_TEST_USER=cf-admin
export SMOKE_TEST_APPS_DOMAIN=vcap.me
export SMOKE_TEST_APP_ROUTE_PROTOCOL=https
export SMOKE_TEST_API_ENDPOINT=https://localhost
export SMOKE_TEST_SKIP_SSL=true

cd "${SCRIPT_DIR}"
ginkgo
