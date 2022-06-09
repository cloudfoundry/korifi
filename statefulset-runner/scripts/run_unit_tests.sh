#!/bin/bash

set -euo pipefail
IFS=$'\n\t'

readonly BASEDIR="$(cd "$(dirname "$0")"/.. && pwd)"
export GO111MODULE=on

main() {
  run_tests
}

run_tests() {
  pushd "$BASEDIR" >/dev/null || exit 1
  go run github.com/onsi/ginkgo/v2/ginkgo --mod=vendor -p -r --keep-going --skip-package=tests --randomize-all --randomize-suites $@
  popd >/dev/null || exit 1
}

main "$@"
