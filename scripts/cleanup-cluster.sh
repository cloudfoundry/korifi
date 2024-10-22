#!/bin/bash

echo "Cleaninig up orgs"
cf curl /v3/organizations | jq -r '.resources[].name' | xargs -n 1 cf delete-org -f

echo "Cleaning up brokers"
cf curl /v3/service_brokers | jq -r '.resources[].name' | xargs -n 1 cf delete-service-broker -f

echo "Clean up offerings and plans"
kubectl delete cfserviceplans.korifi.cloudfoundry.org --all --all-namespaces
kubectl delete cfserviceofferings.korifi.cloudfoundry.org --all --all-namespaces
