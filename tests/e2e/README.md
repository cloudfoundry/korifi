# E2E Tests

## Required Environment Variables
- `API_SERVER_ROOT`: The API URL of the korifi deployment being tested (e.g. "https://localhost").

- `APP_FQDN`: The subdomain that's used for applications pushed to the korifi deployment (e.g. "apps-127-0-0-1.nip.io").

- `ROOT_NAMESPACE`: The root namespace for the korifi deployment (e.g. "cf").

## Optional Environment Variables
- `CLUSTER_TYPE`: The type of kubernetes cluster where korifi is deployed (e.g. "EKS", "GKE").

- `DEFAULT_APP_BITS_PATH`: path to the source code for the application pushed by most tests (by default, the "dorifi" app
  is used).

- `DEFAULT_APP_RESPONSE`: the expected response when curling the default app's root endpoint (by default, "Hi, I'm
  Dorifi").

- `DEFAULT_APP_SPECIFIED_BUILDPACK`: the buildpack to use when pushing the default app with a set buildpack (by default, "
  paketo-buildpacks/procfile").

- `FULL_LOG_ON_ERR`:  If set to a non-blank value, logs for all API requests will be displayed when a test fails.
  Otherwise, only logs that match the current test's correlation ID will be shown.
