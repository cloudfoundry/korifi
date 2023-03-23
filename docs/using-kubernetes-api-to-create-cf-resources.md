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
  displayName: myOrg
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
  displayName: mySpace
EOF

kubectl wait --for=condition=ready cfspace/my-space-guid -n my-org-guid
```
> **Note:** `CFSpace` objects can only be created within the `CFOrg` namespace

Once `CFSpace` is `ready`, you can proceed to grant users access to this space.

### Grant users or service accounts access to Organizations and Spaces

Korifi relies on Kubernetes RBAC (`Roles`, `ClusterRoles`, `RoleBindings`) for authentication and authorization. On the Korifi cluster, [Cloud Foundry roles](https://docs.cloudfoundry.org/concepts/roles.html) (such as `Admin`, `SpaceDeveloper`) are available as [ClusterRoles](https://github.com/cloudfoundry/korifi/tree/main/helm/korifi/controllers/cf_roles).
Users or ServiceAccounts can be granted access by assigning them to these roles through namespace-scoped `RoleBindings`.

> **Note:** Currently, we support only the [Admin](https://github.com/cloudfoundry/korifi/blob/main/helm/korifi/controllers/cf_roles/cf_admin.yaml) and [SpaceDeveloper](https://github.com/cloudfoundry/korifi/blob/main/helm/korifi/controllers/cf_roles/cf_space_developer.yaml) roles.

All Korifi users and service accounts must have a binding to [`cf_user`](https://github.com/cloudfoundry/korifi/blob/main/helm/korifi/controllers/cf_roles/cf_root_namespace_user.yaml) role in the `$ROOT_NAMESPACE`.
This is required
- for Korifi to be able to determine whether a user is registered to use Korifi
- to allow registered users to list `domains`, `orgs`, `buildinfos` (as those are stored in the `$ROOT_NAMESPACE` and should be listable by all registered users with any roles).

To create a `cf_user` rolebinding for a user in the `$ROOT_NAMESPACE`

```sh
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  annotations:
    cloudfoundry.org/propagate-cf-role: "false"
  labels:
    cloudfoundry.org/role-guid: my-cf-user
  name: my-cf-root-user
  namespace: cf
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-root-namespace-user
subjects:
  - kind: User
    name: my-cf-user
EOF

```
> **Note:** When configuring a `RoleBinding`, it is possible to specify multiple `subjects` for a single binding. However, to maintain compatibility with CF CLI it is necessary to maintain a 1:1 ratio between `RoleBindings` and `Users`/`ServiceAccounts`.

To assign a `SpaceDeveloper` role for a user in a space

```sh
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  annotations:
    cloudfoundry.org/propagate-cf-role: "false"
  labels:
    cloudfoundry.org/role-guid: my-cf-user
  name: my-cf-space-user
  namespace: my-space-guid
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-space-developer
subjects:
  - kind: User
    name: my-cf-user
EOF

```

In Kubernetes, `RoleBindings` are namespace-scoped, which means that they are only valid within the `namespace` they were created in. Hence, in the case of `Admin` user, they need to have rolebindings present in all current and future `orgs` and `spaces`.
To make this easier for operators, we have `cloudfoundry.org/propagate-cf-role` annotation that operators can set to `true` on a rolebindings that they want propagated into all child spaces. In case of `Admin`, operators can create an admin rolebinding in `$ROOT_NAMESPACE` and have it propagated to all orgs and spaces.

To assign a `Admin` role for a user in a space

```sh
cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  annotations:
    cloudfoundry.org/propagate-cf-role: "true"
  labels:
    cloudfoundry.org/role-guid: my-cf-user
  name: my-cf-root-user
  namespace: cf
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-admin
subjects:
  - kind: User
    name: my-cf-user
EOF

```

## Using `kapp` to declaratively apply all resources in a single `yaml`. 

[`kapp`](https://carvel.dev/kapp/) is an open source kubernetes deployment tool that provides a simpler and more streamlined way to manage and deploy kubernetes applications using declarative configuration.
See `kapp` [documentation](https://carvel.dev/kapp/docs/v0.54.0/) for installation and usage instructions

To create an `CFOrg`, `CFSpace` & and `Admin` RoleBinding for a user, 

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
  displayName: myOrg
  
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
  displayName: mySpace

---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  annotations:
    cloudfoundry.org/propagate-cf-role: "false"
    kapp.k14s.io/change-rule: "upsert after upserting cfspaces"
  labels:
    cloudfoundry.org/role-guid: my-cf-user
  name: my-cf-root-user
  namespace: cf
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
  annotations:
    cloudfoundry.org/propagate-cf-role: "true"
    kapp.k14s.io/change-rule: "upsert after upserting cfspaces"
  labels:
    cloudfoundry.org/role-guid: my-cf-user
  name: my-cf-admin-user
  namespace: cf
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: korifi-controllers-admin
subjects:
  - kind: User
    name: my-cf-user
EOF
```

