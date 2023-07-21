#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

export SMOKE_TEST_USER="${SMOKE_TEST_USER:-cf-admin}"
export SMOKE_TEST_APPS_DOMAIN="${SMOKE_TEST_APPS_DOMAIN:-apps-127-0-0-1.nip.io}"
export SMOKE_TEST_APP_ROUTE_PROTOCOL="${SMOKE_TEST_APP_ROUTE_PROTOCOL:-https}"
export SMOKE_TEST_API_ENDPOINT="${SMOKE_TEST_API_ENDPOINT:-https://localhost}"
export SMOKE_TEST_SKIP_SSL="${SMOKE_TEST_SKIP_SSL:-true}"

pushd "${SCRIPT_DIR}"
{
  go run github.com/onsi/ginkgo/v2/ginkgo
}
popd
