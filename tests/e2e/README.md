# E2E Tests

## Required Environment Variables
- `API_SERVER_ROOT`: The API URL of the korifi deployment being tested (e.g. "https://localhost").

- `APP_FQDN`: The subdomain that's used for applications pushed to the korifi deployment (e.g. "apps-127-0-0-1.nip.io").

- `CLUSTER_VERSION_MAJOR`: The major version of the kubernetes cluster where korifi is deployed (e.g. `1` for version `1.22`).

- `CLUSTER_VERSION_MINOR`: The minor version of the kubernetes cluster where korifi is deployed (e.g. `22` for version `1.22`).

- `E2E_LONGCERT_USER_NAME`: The username for a user with a certificate that expires far in the future. Used in tests that
  ensure the user is warned about the security implications of their certificate expiration.

- `E2E_SERVICE_ACCOUNT`: The name of a service account that exists in the korifi root namespace. Will be used to test
  service account auth (e.g. "my-service-account").

- `E2E_SERVICE_ACCOUNT_TOKEN`: The token for the E2E_SERVICE_ACCOUNT service account.

- `E2E_UNPRIVILEGED_SERVICE_ACCOUNT`: The name of a service account that exists in the korifi root namespace. Will be used to test
 auth for service accounts that have no korifi privileges (e.g. "my-unprivileged-service-account").

- `E2E_UNPRIVILEGED_SERVICE_ACCOUNT_TOKEN`: The token for the E2E_UNPRIVILEGED_SERVICE_ACCOUNT service account.

- `E2E_USER_NAME`: The name of a user on the kubernetes cluster where korifi is deployed. It can authenticate using either
  a certificate or token. See the E2E_USER_PEM and E2E_USER_TOKEN below. 
 
- `ROOT_NAMESPACE`: The root namespace for the korifi deployment (e.g. "cf").

## Optional Environment Variables
- `CF_ADMIN_PEM`: The certificate PEM for a korifi admin user. Either CF_ADMIN_PEM or CF_ADMIN_TOKEN must be provided.

- `CF_ADMIN_TOKEN`: The token for a korifi admin user. Either CF_ADMIN_PEM or CF_ADMIN_TOKEN must be provided.

- `CLUSTER_TYPE`: The type of kubernetes cluster where korifi is deployed (e.g. "EKS", "GKE").

- `DEFAULT_APP_BITS_PATH`: path to the source code for the application pushed by most tests (by default, the "dorifi" app
  is used).

- `DEFAULT_APP_RESPONSE`: the expected response when curling the default app's root endpoint (by default, "Hi, I'm
  Dorifi").

- `DEFAULT_APP_SPECIFIED_BUILDPACK`: the buildpack to use when pushing the default app with a set buildpack (by default, "
  paketo-buildpacks/procfile").

- `E2E_LONGCERT_USER_PEM`: The certificate PEM for the E2E_LONGCERT_USER_NAME user. If this var is omitted, then some
  tests will be skipped.

- `E2E_USER_PEM`: The certificate PEM for the E2E_USER_NAME user. If this var is omitted, then some tests will be skipped.
  Either E2E_USER_PEM or E2E_USER_TOKEN must be provided.

- `E2E_USER_TOKEN`: The token for the E2E_USER_NAME user. Either E2E_USER_PEM or E2E_USER_TOKEN must be provided.

- `FULL_LOG_ON_ERR`:  If set to a non-blank value, logs for all API requests will be displayed when a test fails.
  Otherwise, only logs that match the current test's correlation ID will be shown.
