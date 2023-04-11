# Using Kubernetes API to create CF resources

Korifi is backed entirely by Kubernetes [Custom Resources](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/), enabling operators to manage organizations, spaces and roles declaratively through the Kubernetes API. 
Operators can use any K8s API client(such as `kubectl`, `kapp` etc) to manage resources. We have documented examples using both clients. In the examples below we are assuming default value for `$ROOT_NAMESPACE` which is `cf`.

## Using `kubectl` to manage resources

### Creating Orgs

Use `CFOrg` custom resource to create an Organization

```sh
cat <<EOF | kubectl apply -f -
apiVersion: korifi.cloudfoundry.org/v1alpha1
kind: CFOrg
metadata:
  name: my-org-guid
  namespace: cf
spec:
  displayName: my-org
EOF

kubectl wait --for=condition=ready cforg/my-org-guid -n cf
```
> **Note:** `CFOrg` objects can only be created within the `$ROOT_NAMESPACE`

Once `CFOrg` is `ready`, you can proceed to create spaces or grant users access to this organization.

### Creating Spaces

Use `CFSpace` custom resource to create a Space

```sh
cat <<EOF | kubectl apply -f -
apiVersion: korifi.cloudfoundry.org/v1alpha1
kind: CFSpace
metadata:
  name: my-space-guid
  namespace: my-org-guid
spec:
  displayName: my-space
EOF

kubectl wait --for=condition=ready cfspace/my-space-guid -n my-org-guid
```
> **Note:** `CFSpace` objects can only be created within the `CFOrg` namespace

Once `CFSpace` is `ready`, you can proceed to grant users access to this space.

### Grant users or service accounts access to Organizations and Spaces

Korifi relies on Kubernetes RBAC (`Roles`, `ClusterRoles`, `RoleBindings`) for authentication and authorization. On the Korifi cluster, [Cloud Foundry roles](https://docs.cloudfoundry.org/concepts/roles.html) (such as `Admin`, `SpaceDeveloper`) are represented as [ClusterRoles](https://github.com/cloudfoundry/korifi/tree/main/helm/korifi/controllers/cf_roles).
Users or ServiceAccounts are granted access by assigning them these roles through namespace-scoped `RoleBindings`.

> **Note:** Currently, we support only the [Admin](https://github.com/cloudfoundry/korifi/blob/main/helm/korifi/controllers/cf_roles/cf_admin.yaml),
> [OrgUser](https://github.com/cloudfoundry/korifi/blob/main/helm/korifi/controllers/cf_roles/cf_org_user.yaml),
> and [SpaceDeveloper](https://github.com/cloudfoundry/korifi/blob/main/helm/korifi/controllers/cf_roles/cf_space_developer.yaml) roles.

#### Grant a user or service account access to the Korifi API
All Korifi users and service accounts must have a `RoleBinding` in the `$ROOT_NAMESPACE` for the `ClusterRole` `korifi-controllers-root-namespace-user` to access the API.

This is required
- to determine whether a user is allowed access to Korifi
- to allow registered users to list `CFDomains`, `CFOrgs`, and `BuilderInfos` resources. These are stored in the `$ROOT_NAMESPACE` and should be listable by all registered users with any roles.

Here's an example command to create this role binding for a user named "my-cf-user":

```sh
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-cf-user-korifi-access
  namespace: cf
  labels:
    cloudfoundry.org/role-guid: my-cf-user-korifi-access-guid
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-root-namespace-user
subjects:
  - kind: User
    name: my-cf-user
EOF
```

Here's an example command to create this role binding for a service account named "my-service-account" in namespace "my-service-account-namespace":

```sh
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-service-account-korifi-access
  namespace: cf
  labels:
    cloudfoundry.org/role-guid: my-service-account-korifi-access-guid
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-root-namespace-user
subjects:
  - kind: ServiceAccount
    name: my-service-account
    namespace: my-service-account-namespace
EOF
```

#### Grant a user or service account admin-level access
In Kubernetes, `RoleBindings` are namespace-scoped, which means that they are only valid within the namespace they were created in.
In the case of an admin user, a rolebindings to the `korifi-contollers-admin` `ClusterRole` is required in the `$ROOT_NAMESPACE`, as well as the namespaces for all current and future orgs and spaces.
To make this easier for operators, we have the `cloudfoundry.org/propagate-cf-role=true` annotation for rolebindings in the `$ROOT_NAMESPACE`.
This annotation will propagate them into the namespaces that represent all orgs and spaces.

Here's an example of assigning the admin role to the user "my-cf-user":

```sh
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-cf-user-admin
  namespace: cf
  labels:
    cloudfoundry.org/role-guid: my-cf-user-admin-guid
  annotations:
    cloudfoundry.org/propagate-cf-role: "true"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-admin
subjects:
  - kind: User
    name: my-cf-user
EOF
```

Here's an example of assigning the admin role to the service account "my-service-account" in namespace "my-service-account-namespace":

```sh
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-service-account-admin
  namespace: cf
  labels:
    cloudfoundry.org/role-guid: my-service-account-admin-guid
  annotations:
    cloudfoundry.org/propagate-cf-role: "true"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-admin
subjects:
  - kind: ServiceAccount
    name: my-service-account
    namespace: my-service-account-namespace
EOF
```

#### Granting a user or service account space developer access
If you only want to grant a user `SpaceDeveloper` access, you can instead create a `RoleBinding` to the `ClusterRole` `korifi-controllers-root-namespace-user` in a space's namespace.

Here's an example of assigning the `SpaceDeveloper` CF role to the user "my-cf-user" in the space with GUID "my-space-guid"

```sh
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-cf-user-space-developer
  namespace: my-space-guid
  labels:
    cloudfoundry.org/role-guid: my-cf-user-space-developer-guid
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-space-developer
subjects:
  - kind: User
    name: my-cf-user
EOF
```

Here's an example of assigning the `SpaceDeveloper` CF role to the service account "my-service-account" in namespace "my-service-account-namespace":

```sh
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-service-account-space-developer
  namespace: my-space-guid
  labels:
    cloudfoundry.org/role-guid: my-service-account-space-developer-guid
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-space-developer
subjects:
  - kind: ServiceAccount
    name: my-service-account
    namespace: my-service-account-namespace
EOF
```

All non-admin users must also be `OrgUser`s of the org that contains the space. This is represented by a `RoleBinding`
to the `ClusterRole` `korifi-controllers-organization-user` in the org's namespace.

Here's an example of assigning the `OrgUser` CF role to the user "my-cf-user" in the org with GUID "my-org-guid"

```sh
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-cf-user-org-user
  namespace: my-org-guid
  labels:
    cloudfoundry.org/role-guid: my-cf-user-org-user-guid
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-organization-user
subjects:
  - kind: User
    name: my-cf-user
EOF
```

Here's an example of assigning the `OrgUser` CF role to the service account "my-service-account" in namespace "my-service-account-namespace":

```sh
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-service-account-org-user
  namespace: my-org-guid
  labels:
    cloudfoundry.org/role-guid: my-service-account-org-user-guid
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-organization-user
subjects:
  - kind: ServiceAccount
    name: my-service-account
    namespace: my-service-account-namespace
EOF
```

> **Note:** When configuring a `RoleBinding`, it is possible to specify multiple `subjects` for a single binding. However, to maintain compatibility with CF CLI it is necessary to maintain a 1:1 ratio between `RoleBindings` and `Users`/`ServiceAccounts`.

## Using `kapp` to declaratively apply all resources in a single `yaml`. 

[`kapp`](https://carvel.dev/kapp/) is an open source kubernetes deployment tool that provides a simpler and more streamlined way to manage and deploy kubernetes applications using declarative configuration.
See `kapp` [documentation](https://carvel.dev/kapp/docs/v0.54.0/) for installation and usage instructions

Here's an example of creating a `CFOrg`, `CFSpace` and granting the user "my-cf-user" the CF `Admin` role in a single command:

```shell
cat <<EOF | kapp deploy -a my-config -y -f -
---
apiVersion: kapp.k14s.io/v1alpha1
kind: Config
metadata:
  name: kapp-config
  annotations: {}
  
minimumRequiredVersion: 0.29.0

waitRules:
- supportsObservedGeneration: false
  conditionMatchers:
  - type: Ready
    status: "False"
    failure: true
  - type: Ready
    status: "True"
    success: true
  resourceMatchers:
  - apiVersionKindMatcher: {apiVersion: korifi.cloudfoundry.org/v1alpha1, kind: CFOrg}
  - apiVersionKindMatcher: {apiVersion: korifi.cloudfoundry.org/v1alpha1, kind: CFSpace}
  
---
apiVersion: korifi.cloudfoundry.org/v1alpha1
kind: CFOrg
metadata:
  name: my-org-guid
  namespace: cf
  annotations:
    kapp.k14s.io/change-group: "cforgs"
spec:
  displayName: my-org
  
---
apiVersion: korifi.cloudfoundry.org/v1alpha1
kind: CFSpace
metadata:
  name: my-space-guid
  namespace: my-org-guid
  annotations:
    kapp.k14s.io/change-group: "cfspaces"
    kapp.k14s.io/change-rule: "upsert after upserting cforgs"
spec:
  displayName: my-space

---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-cf-user-korifi-access
  namespace: cf
  labels:
    cloudfoundry.org/role-guid: my-cf-user-korifi-access-guid
  annotations:
    kapp.k14s.io/change-rule: "upsert after upserting cfspaces"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-root-namespace-user
subjects:
  - kind: User
    name: my-cf-user

---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-cf-user-admin
  namespace: cf
  labels:
    cloudfoundry.org/role-guid: my-cf-user-admin-guid
  annotations:
    cloudfoundry.org/propagate-cf-role: "true"
    kapp.k14s.io/change-rule: "upsert after upserting cfspaces"
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-admin
subjects:
  - kind: User
    name: my-cf-user
EOF
```

