#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

export CRDS_TEST_API_ENDPOINT=https://localhost
export CRDS_TEST_SKIP_SSL=true
export CRDS_TEST_CLI_USER=crd-cli-test-user

"${SCRIPT_DIR}/../../scripts/create-new-user.sh" "$CRDS_TEST_CLI_USER"

cd "${SCRIPT_DIR}"
go run github.com/onsi/ginkgo/v2/ginkgo
