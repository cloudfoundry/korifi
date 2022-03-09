#!/usr/bin/env bash

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

pushd "${SCRIPT_DIR}"
{
  cf api https://localhost --skip-ssl-validation
  cf login <<EOF
1
1
1
EOF
  SMOKE_TEST_APP_ROUTE_PROTOCOL="http" SMOKE_TEST_APPS_DOMAIN="vcap.me" go test
}
popd
