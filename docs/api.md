# Korifi API Documentation

This document lists all the CF API endpoints supported by Korifi and their parameters.

## [Apps](https://v3-apidocs.cloudfoundry.org/#apps)

### [Create an app](https://v3-apidocs.cloudfoundry.org/#create-an-app)

#### Supported parameters:

All parameters are supported. `lifecycle` will be ignored and overridden with the default configured values.

### [Get an app](https://v3-apidocs.cloudfoundry.org/#get-an-app)

#### Supported query parameters:

No query parameters are supported.

### [List apps](https://v3-apidocs.cloudfoundry.org/#list-apps)

#### Supported query parameters:

-   `names`
-   `space_guids`
-   `order_by` (the only supported value is `name`)

### [Delete an app](https://v3-apidocs.cloudfoundry.org/#delete-an-app)

This endpoint is fully supported.

### [Get current droplet](https://v3-apidocs.cloudfoundry.org/#get-current-droplet)

This endpoint is fully supported.

### [Get environment for an app](https://v3-apidocs.cloudfoundry.org/#get-environment-for-an-app)

> **Warning**
> The field `system_env_json` will **not** be redacted.

### [Set current droplet](https://v3-apidocs.cloudfoundry.org/#update-a-droplet)

This endpoint is fully supported.

### [Start an app](https://v3-apidocs.cloudfoundry.org/#start-an-app)

This endpoint is fully supported.

### [Stop an app](https://v3-apidocs.cloudfoundry.org/#stop-an-app)

This endpoint is fully supported.

### [Restart an app](https://v3-apidocs.cloudfoundry.org/#restart-an-app)

This endpoint is fully supported.

### [Update environment variables for an app](https://v3-apidocs.cloudfoundry.org/#update-environment-variables-for-an-app)

This endpoint is fully supported.

## [Builds](https://v3-apidocs.cloudfoundry.org/#builds)

### [Create a build](https://v3-apidocs.cloudfoundry.org/#create-a-build)

All parameters are supported. `lifecycle` will be ignored and overridden with the default configured values.

### [Get a build](https://v3-apidocs.cloudfoundry.org/#get-a-build)

This endpoint is fully supported.

## [Buildpacks](https://v3-apidocs.cloudfoundry.org/#buildpacks)

### [List buildpacks](https://v3-apidocs.cloudfoundry.org/#list-buildpacks)

#### Supported query parameters:

No query parameters are supported.

## [Organizations](https://v3-apidocs.cloudfoundry.org/#organizations)

### [Create an organization](https://v3-apidocs.cloudfoundry.org/#create-an-organization)

#### Supported parameters:

-   `name`

### [List organizations](https://v3-apidocs.cloudfoundry.org/#list-organizations)

#### Supported query parameters:

-   `names`

### [Delete an organization](https://v3-apidocs.cloudfoundry.org/#delete-an-organization)

This endpoint is fully supported.

## [Domains](https://v3-apidocs.cloudfoundry.org/#domains)

### [List Domains](https://v3-apidocs.cloudfoundry.org/#list-domains)

#### Supported query parameters:

-   `names`

### [List domains for an organization](https://v3-apidocs.cloudfoundry.org/#list-domains-for-an-organization)

#### Supported query parameters:

-   `names`

## [Droplets](https://v3-apidocs.cloudfoundry.org/#droplets)

### [Get a droplet](https://v3-apidocs.cloudfoundry.org/#get-a-droplet)

> **Warning**
> No fields will be redacted.

### [List droplets for a package](https://v3-apidocs.cloudfoundry.org/#list-droplets-for-a-package)

#### Supported query parameters:

No query parameters are supported.

## [Jobs](https://v3-apidocs.cloudfoundry.org/#jobs)

### [Get a job](https://v3-apidocs.cloudfoundry.org/#get-a-job)

> **Warning**
> This endpoint always returns an empty resource with `state: "COMPLETE"`.

## [Manifests](https://v3-apidocs.cloudfoundry.org/#manifests)

### [Apply a manifest to a space](https://v3-apidocs.cloudfoundry.org/#apply-a-manifest-to-a-space)

Korifi only supports manifests with a single entry in `applications`.

#### Supported parameters:

-   `applications[0].name`
-   `applications[0].env`
-   `applications[0].memory` (sets `memory` for the `web` process)
-   `applications[0].processes`
-   `applications[0].no-route`
-   `applications[0].routes[0].route`

### [Create a manifest diff for a space](https://v3-apidocs.cloudfoundry.org/#create-a-manifest-diff-for-a-space-experimental)

> **Warning**
> This endpoint always returns an empty diff.

## [Packages](https://v3-apidocs.cloudfoundry.org/#packages)

### [Create a package](https://v3-apidocs.cloudfoundry.org/#create-a-package)

#### Supported parameters:

-   `type` (the only supported value is `bits`)
-   `relationships.app`

### [Get a package](https://v3-apidocs.cloudfoundry.org/#get-a-package)

This endpoint is fully supported.

### [List Package](https://v3-apidocs.cloudfoundry.org/#list-packages)

#### Supported query parameters:

-   `app_guids`
-   `states`
-   `order_by` (the only supported value is `created_at`)

### [Upload package bits](https://v3-apidocs.cloudfoundry.org/#upload-package-bits)

#### Supported parameters:

-   `bits`

## [Processes](https://v3-apidocs.cloudfoundry.org/#processes)

### [Get a process](https://v3-apidocs.cloudfoundry.org/#get-a-process)

These endpoints are fully supported.

### [Get stats for a process](https://v3-apidocs.cloudfoundry.org/#get-stats-for-a-process)

`GET /v3/apps/:guid/processes/:type/stats` is not supported.

#### Supported fields:

-   `index`
-   `state`

### [List processes](https://v3-apidocs.cloudfoundry.org/#list-processes)

#### Supported query parameters:

-   `app_guids`

### [List processes for app](https://v3-apidocs.cloudfoundry.org/#list-processes-for-app)

#### Supported query parameters:

No query parameters are supported.

### [Update a process](https://v3-apidocs.cloudfoundry.org/#update-a-process)

#### Supported parameters:

-   `command`
-   `health_check`

### [Scale a process](https://v3-apidocs.cloudfoundry.org/#scale-a-process)

This endpoint is fully supported.

## [Resource Matches](https://v3-apidocs.cloudfoundry.org/#resource-matches)

### [Create a resource match](https://v3-apidocs.cloudfoundry.org/#create-a-resource-match)

> **Warning**
> CF for VMs uses a technique called "resource matching" as an optimization to support partial app uploads to the blobstore. Korifi does not support this feature and this endpoint will always return an empty list of matched resources.

## [Roles](https://v3-apidocs.cloudfoundry.org/#roles)

### [Create a role](https://v3-apidocs.cloudfoundry.org/#create-a-role)

#### Supported parameters:

-   `type` (the only supported value is `space_developer`
-   `relationships.user`
-   `relationships.organization`
-   `relationships.space`

## [Root](https://v3-apidocs.cloudfoundry.org/#root)

### [Global API Root](https://v3-apidocs.cloudfoundry.org/#global-api-root)

#### Supported fields:

-   `self`
-   `cloud_controller_v3`
-   `login`
-   `log_cache`

### [V3 API Root](https://v3-apidocs.cloudfoundry.org/#v3-api-root)

#### Supported fields:

-   `links.self`

## [Routes](https://v3-apidocs.cloudfoundry.org/#routes)

### [Create a route](https://v3-apidocs.cloudfoundry.org/#create-a-route)

#### Supported parameters:

-   `relationships.space`
-   `relationships.domain`
-   `host`
-   `path`
-   `metadata.annotations`
-   `metadata.labels`

### [Get a route](https://v3-apidocs.cloudfoundry.org/#get-a-route)

#### Supported query parameters:

No query parameters are supported.

### [List routes](https://v3-apidocs.cloudfoundry.org/#list-routes)

#### Supported query parameters:

-   `app_guids`
-   `space_guids`
-   `domain_guids`
-   `hosts`
-   `paths`

### [List routes for an app](https://v3-apidocs.cloudfoundry.org/#list-routes-for-an-app)

#### Supported query parameters:

No query parameters are supported.

### [Delete a route](https://v3-apidocs.cloudfoundry.org/#delete-a-route)

This endpoint is fully supported.

### [List destinations for a route](https://v3-apidocs.cloudfoundry.org/#list-destinations-for-a-route)

#### Supported query parameters:

No query parameters are supported.

### [Insert destinations for a route](https://v3-apidocs.cloudfoundry.org/#insert-destinations-for-a-route)

#### Supported parameters:

-   `destinations[].app.guid`
-   `destinations[].app.process.type`
-   `destinations[].port`
-   `destinations[].protocol`

### [Remove destination for a route](https://v3-apidocs.cloudfoundry.org/#remove-destination-for-a-route)

This endpoint is fully supported.

## [Service Instances](https://v3-apidocs.cloudfoundry.org/#service-instances)

Korifi only supports user-provided service instances. Managed service operations and [fields](https://v3-apidocs.cloudfoundry.org/#fields) are not supported.

### [Create a service instance](https://v3-apidocs.cloudfoundry.org/#create-a-service-instance)

#### Supported parameters:

-   `type` (must be `user-provided`)
-   `name`
-   `relationships.space`
-   `tags`
-   `credentials`
-   `metadata.labels`
-   `metadata.annotations`

### [List service instances](https://v3-apidocs.cloudfoundry.org/#list-service-instances)

#### Supported query parameters:

-   `names`
-   `space_guids`
-   `order_by` (the only supported values are `name`, `created_at` and `updated_at`)

### [Delete a service instance](https://v3-apidocs.cloudfoundry.org/#delete-a-service-instance)

#### Supported query parameters:

No query parameters are supported.

## [Service Credential Bindings](https://v3-apidocs.cloudfoundry.org/#service-credential-binding)

### [Create a service credential binding](https://v3-apidocs.cloudfoundry.org/#create-a-service-credential-binding)

#### Supported parameters:

-   `name`
-   `type` (the only supported value is `app`)
-   `relationships.service_instance`
-   `relationships.app`

### [List service credential bindings](https://v3-apidocs.cloudfoundry.org/#list-service-credential-bindings)

#### Supported query parameters:

-   `service_instance_guids`
-   `app_guids`
-   `type`
-   `include` (the only supported value is `app`)

### [Delete a service credential binding](https://v3-apidocs.cloudfoundry.org/#delete-a-service-credential-binding)

This endpoint is fully supported.

## [Service Route Bindings](https://v3-apidocs.cloudfoundry.org/#service-route-binding)

### [List service route bindings](https://v3-apidocs.cloudfoundry.org/#list-service-route-bindings)

> **Warning**
> This endpoint always returns an empty list.

## [Sidecars](https://v3-apidocs.cloudfoundry.org/#sidecars)

### [List sidecars for process](https://v3-apidocs.cloudfoundry.org/#list-sidecars-for-process)

> **Warning**
> This endpoint always returns an empty list.

## [Spaces](https://v3-apidocs.cloudfoundry.org/#spaces)

### [Create a space](https://v3-apidocs.cloudfoundry.org/#create-a-space)

#### Supported parameters:

-   `name`
-   `relationships.guid`

### [List spaces](https://v3-apidocs.cloudfoundry.org/#list-spaces)

#### Supported query parameters:

-   `names`
-   `organization_guids`

### [Delete a space](https://v3-apidocs.cloudfoundry.org/#delete-a-space)

This endpoint is fully supported.

## User Identity

> **Warning**
> This is not part of the published CF API, and is not supported on CF on VMs.

These endpoints are needed by the CF CLI when targeting a Korifi instance to properly identify the user.

### Get the current username

#### Definition

```
GET /whoami
```

## [Log-Cache](https://github.com/cloudfoundry/log-cache)

### [Info](https://github.com/cloudfoundry/log-cache#get-apiv1info)

> **Warning**
> This endpoint is unauthenticated.
> This endpoint will always return a hard-coded response.

### [Read](https://github.com/cloudfoundry/log-cache#get-apiv1readsource-id)

#### Supported query parameters:

-   `start_time`
-   `limit`
-   `descending`
