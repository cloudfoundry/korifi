#!/bin/bash

set -eux

readonly TMP_DIR=/workspace
mkdir -p "$TMP_DIR"
readonly EIRINI_DIR="$TMP_DIR/eirini"

cp -a /eirini "$TMP_DIR"

pushd "$EIRINI_DIR"
{
  $TEST_SCRIPT
}
popd
