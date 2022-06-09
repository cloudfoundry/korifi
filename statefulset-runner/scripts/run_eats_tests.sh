#!/bin/bash

set -euo pipefail

readonly BASEDIR="$(cd "$(dirname "$0")"/.. && pwd)"
export GO111MODULE=on
if [ -z ${EIRINIUSER_PASSWORD+x} ]; then
  EIRINIUSER_PASSWORD="$(pass eirini/docker-hub)"
fi

main() {
  pushd "$BASEDIR"/tests/eats >/dev/null || exit 1
  go run github.com/onsi/ginkgo/v2/ginkgo --mod=vendor -p -r --keep-going --randomize-all --randomize-suites --timeout=20m --slow-spec-threshold=30s "$@"
  popd >/dev/null || exit 1
}

main "$@"
