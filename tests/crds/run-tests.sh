#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

export CRDS_TEST_API_ENDPOINT="${CRDS_TEST_API_ENDPOINT:-https://localhost}"
export CRDS_TEST_SKIP_SSL="${CRDS_TEST_SKIP_SSL:-true}"
export CRDS_TEST_CLI_USER="${CRDS_TEST_CLI_USER:-}"

if [[ -z "${CRDS_TEST_CLI_USER:-}" ]]; then
  export CRDS_TEST_CLI_USER="crd-cli-test-user"
  "${SCRIPT_DIR}/../../scripts/create-new-user.sh" "$CRDS_TEST_CLI_USER"
fi

pushd "${SCRIPT_DIR}"
{
  go run github.com/onsi/ginkgo/v2/ginkgo
}
popd
