#!/bin/bash

set -euo pipefail

readonly BASEDIR="$(cd "$(dirname "$0")"/.. && pwd)"
export GO111MODULE=on
if [ -z ${EIRINIUSER_PASSWORD+x} ]; then
  EIRINIUSER_PASSWORD="$(pass eirini/docker-hub)"
fi

nodes="-p"
if [[ "${NODES:-}" != "" ]]; then
  nodes="--nodes $NODES"
fi

main() {
  export EIRINI_BINS_PATH
  EIRINI_BINS_PATH=$(mktemp -d)
  trap "rm -rf $EIRINI_BINS_PATH" EXIT

  pushd "$BASEDIR"/tests/integration >/dev/null || exit 1
  {
    go run github.com/onsi/ginkgo/v2/ginkgo --mod=vendor $nodes -r --keep-going --tags=integration --randomize-all --randomize-suites --timeout=20m --slow-spec-threshold=25s $@
  }
  popd >/dev/null || exit 1
}

main "$@"
