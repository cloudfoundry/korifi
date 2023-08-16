#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

export CRDS_TEST_API_ENDPOINT="${CRDS_TEST_API_ENDPOINT:-https://localhost}"
export CRDS_TEST_SKIP_SSL="${CRDS_TEST_SKIP_SSL:-true}"

pushd "${SCRIPT_DIR}"
{
  go run github.com/onsi/ginkgo/v2/ginkgo
}
popd
