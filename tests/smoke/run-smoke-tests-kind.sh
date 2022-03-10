#!/usr/bin/env bash

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

pushd "${SCRIPT_DIR}"
{
  cf api https://localhost --skip-ssl-validation
  cf login << EOF
2
2
2
EOF
  SMOKE_TEST_APP_ROUTE_PROTOCOL="https" SMOKE_TEST_APPS_DOMAIN="vcap.me" go test
}
popd
