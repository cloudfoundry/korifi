#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

source "$SCRIPT_DIR/common.sh"

if [[ $# -ne 1 ]]; then
  cat <<EOF >&2
Usage:
  $(basename "$0") <username>

EOF
  exit 1
fi

username="$1"
tmp="$(mktemp -d)"
trap "rm -rf $tmp" EXIT

createCert $username $tmp/key.pem $tmp/cert.pem

kubectl config set-credentials \
  "${username}" \
  --client-certificate="$tmp/cert.pem" \
  --client-key="$tmp/key.pem" \
  --embed-certs

cat <<EOF

Use "cf set-space-role ${username} ORG SPACE SpaceDeveloper" to grant this user permissions in a space.
EOF
