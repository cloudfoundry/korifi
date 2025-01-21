# UAA authentication

## Overview

In CF for VMs the `cf cli` authenticates the user against the CF UAA instance when performing `cf login`. The token returned by the UAA is supplied in subsequent requests to the CF API in the `Authorization` request header.

In contrast, Korifi authenticates to the Kubernetes cluster using the credentials supplied in the user's kubeconfig file. Users may find this confusing as they need to both configure the cf api (via `cf api`) AND their kubeconfig file. Furthermore, operators may feel uncomfortable giving access to the kubernetes cluster to their users.

With Korifi 0.14.0 we introduce the experimental `experimental.uaa` helm values that allow Korifi users to authenticate against a UAA instance.

## Configuration

### Cluster configuration

The cluster needs to be configured with the UAA instance as [OpenID Connect (OIDC)](https://auth0.com/docs/authenticate/protocols/openid-connect-protocol) identity provider. Various hyperscalers (e.g. [GKE](https://cloud.google.com/kubernetes-engine/docs/how-to/oidc), [EKS](https://docs.aws.amazon.com/eks/latest/userguide/authenticate-oidc-identity-provider.html)) have different means to achieve that.

Below is the configuration you could use when creating a kind cluster to use a CF for VMs UAA:

* Download the UAA instance certificate to `/path/to/uaa.pem`
* The snippets below assume the following environment variables are set:

```
export ROOT_NAMESPACE="cf"
export UAA_URL="https://my.uaa.com"
export OIDC_PREFIX="uaa"
export UAA_PEM="/path/to/uaa.pem"
export ADMIN_EMAIL="your.user@company.com"
```

* Create the kind cluster with the following configuration:

```bash
cat <<EOF | kind create cluster --name korifi --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraMounts:
  - containerPath: /ssl
    hostPath: $(dirname ${UAA_PEM})
    readOnly: true
  kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
      extraVolumes:
        - name: ssl-certs
          hostPath: /ssl
          mountPath: /etc/uaa-ssl
      extraArgs:
        oidc-issuer-url: ${UAA_URL}/oauth/token
        oidc-client-id: cloud_controller
        oidc-ca-file: /etc/uaa-ssl/uaa.pem
        oidc-username-claim: user_name
        oidc-username-prefix: "${OIDC_PREFIX}:"
        oidc-signing-algs: "RS256"
  extraPortMappings:
  - containerPort: 32080
    hostPort: 80
    protocol: TCP
  - containerPort: 32443
    hostPort: 443
    protocol: TCP
  - containerPort: 30050
    hostPort: 30050
    protocol: TCP
EOF
```

### Korifi configuration

Set the following values on the Korifi helm chart:

* `experimental.uaa.enabled: true`
* `experimental.uaa.url: <uaa-url>`

For example

```
helm install korifi https://github.com/cloudfoundry/korifi/releases/download/v<VERSION>/korifi-<VERSION>.tgz \
    ....
    --set=experimental.uaa.enabled="true" \
    --set=experimental.uaa.url="$UAA_URL" \
    ....
```

## Configure the admin user

Create the following admin role binding for your user:

```bash
kubectl apply -f - <<EOF
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  annotations:
    cloudfoundry.org/propagate-cf-role: "true"
  name: my-user-admin-binding
  namespace: "$ROOT_NAMESPACE"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-admin
subjects:
- apiGroup: rbac.authorization.k8s.io
  kind: User
  name: "$OIDC_PREFIX:$ADMIN_EMAIL"
EOF

```


## Login

As usual, run the `cf login` command. When prompted, provide your UAA user credentials. You could also login using Single SignOn (SSO) via `cf login --sso`

## Creating roles for users
When configuring user roles via the `cf cli`, the OIDC prefix must be specified as `origin`:

```
cf set-space-role "another.user@company.com" org space SpaceDeveloper --origin $OIDC_PREFIX
```

## Limitations

The experimental UAA integration only supports authentication. Authorization is still configured via cluster-local RBAC role bindings.
