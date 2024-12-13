#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ORGS_NUM=1000

cf login -u cf-admin -o org-1

$SCRIPT_DIR/create-new-user.sh "my-user"

for ((i = 1; i <= $ORGS_NUM; i++)); do
  cf create-org "org-$i"
  cf create-space -o "org-$i" "space"
  cf set-space-role "my-user" "org-$i" space SpaceDeveloper
done

cf target -o "org-1" -s "space"
# cf push dorifi -p $SCRIPT_DIR/../tests/assets/dorifi

cf login -u my-user -o org-1

echo ">>> Listing apps"
time cf apps

# echo ">>> kubectl get cfapp with label selector"
# SPACE_GUIDS="$(kubectl get cfspaces.korifi.cloudfoundry.org --all-namespaces -o=custom-columns=NAME:.metadata.name --no-headers | tr '\n' ',')"

# time kubectl get cfapps --all-namespaces -l "korifi.cloudfoundry.org/space-guid in ($SPACE_GUIDS)"
