#!/bin/bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT_DIR="$ROOT_DIR/deployment/scripts"

pass eirini/docker-hub | docker login -u eiriniuser --password-stdin

"$SCRIPT_DIR"/build.sh
"$SCRIPT_DIR"/deploy-only.sh "$@"
