#!/bin/bash

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ROLES_DIR="$ROOT_DIR/api/config/cf_roles"
CONTROLLER_GEN="$ROOT_DIR/controllers/bin/controller-gen"

for rolepath in $ROOT_DIR/api/roles/*; do
  filename=$(basename $rolepath)
  rolefilename=${filename/".go"/}
  rolename=${rolefilename//"_"/"-"}

  "$CONTROLLER_GEN" rbac:roleName="$rolename" paths="$rolepath" output:rbac:stdout >"$ROLES_DIR/cf_${rolefilename}.yaml"

done
