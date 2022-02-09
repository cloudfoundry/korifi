# CF K8s API Endpoint Documentation

**This document captures API endpoints currently supported by the shim**

## Resources

### Root

Docs: https://v3-apidocs.cloudfoundry.org/version/3.110.0/index.html#root

| Resource        | Endpoint |
| --------------- | -------- |
| Global API Root | GET /    |
| V3 API Root     | GET /v3  |

### Resource Matches

Docs: https://v3-apidocs.cloudfoundry.org/version/3.110.0/index.html#resource-matches

| Resource                | Endpoint                  |
| ----------------------- | ------------------------- |
| Create a Resource Match | POST /v3/resource_matches |

#### [Create a Resource Match](https://v3-apidocs.cloudfoundry.org/version/3.110.0/index.html#create-a-resource-match)
```bash
curl "http://localhost:9000/v3/resource_matches" \
  -X POST \
  -d '{}'
```

### Orgs

Docs: https://v3-apidocs.cloudfoundry.org/version/3.110.0/index.html#organizations

| Resource   | Endpoint      |
| ---------- | ------------- |
| List Orgs  | GET /v3/organizations  |
| Create Org | POST /v3/organizations |
| Delete Space | [DELETE /v3/organizations/:guid](https://v3-apidocs.cloudfoundry.org/version/3.113.0/index.html#delete-an-organization)

#### [List Orgs](https://v3-apidocs.cloudfoundry.org/version/3.110.0/index.html#list-organizations)
**Query Parameters:** Currently only supports filtering by organization `names`.
```bash
curl "http://localhost:9000/v3/organizations?names=org1,org2"
```
#### [Creating Orgs](https://v3-apidocs.cloudfoundry.org/version/3.110.0/index.html#create-an-organization)
```bash
curl "http://localhost:9000/v3/organizations" \
  -X POST \
  -d '{"name": "my-organization"}'
```

### Spaces

Docs: https://v3-apidocs.cloudfoundry.org/version/3.110.0/index.html#spaces

| Resource     | Endpoint                                                                                                   |
| ------------ | ---------------------------------------------------------------------------------------------------------- |
| List Spaces  | GET /v3/spaces                                                                                             |
| Create Space | POST /v3/spaces                                                                                            |
| Delete Space | [DELETE /v3/spaces/\<guid>](https://v3-apidocs.cloudfoundry.org/version/3.111.0/index.html#delete-a-space) |

#### [List Spaces](https://v3-apidocs.cloudfoundry.org/version/3.110.0/index.html#list-spaces)
**Query Parameters:** Currently supports filtering by space `names` and `organization_guids`.

```bash
curl "http://localhost:9000/v3/spaces?names=space1,space2&organization_guids=ad0836b5-09f4-48c0-adb2-2c61e515562f,6030b015-f003-4c9f-8bb4-1ed7ae3d3659"
```

#### [Creating Spaces](https://v3-apidocs.cloudfoundry.org/version/3.110.0/index.html#create-a-space)

```bash
curl "http://localhost:9000/v3/spaces" \
  -X POST \
  -d '{"name":"my-space","relationships":{"organization":{"data":{"guid":"<organization-guid>"}}}}'
```

### Apps

Docs: https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#apps

| Resource                            | Endpoint                                                                                                |
|-------------------------------------|---------------------------------------------------------------------------------------------------------|
| List Apps                           | GET /v3/apps                                                                                            |
| Get App                             | GET /v3/apps/\<guid>                                                                                    |
| Create App                          | POST /v3/apps                                                                                           |
| Set App's Current Droplet           | PATCH /v3/apps/\<guid>/relationships/current_droplet                                                    |
| Get App's Current Droplet           | GET /v3/apps/\<guid>/droplets/current                                                                   |
| Start App                           | POST /v3/apps/\<guid>/actions/start                                                                     |
| Stop App                            | POST /v3/apps/\<guid>/actions/stop                                                                      |
| Restart App                         | POST /v3/apps/\<guid>/actions/restart                                                                   |
| List App Processes                  | GET /v3/apps/\<guid>/processes                                                                          |
| Scale App Process                   | POST /v3/apps/<guid>/processes/<type>/actions/scale                                                     |
| List App Routes                     | GET /v3/apps/\<guid>/routes                                                                             |
| Delete App                          | [DELETE /v3/apps/\<guid>](https://v3-apidocs.cloudfoundry.org/version/3.111.0/index.html#delete-an-app) |
 | Get App Env                         | GET /v3/apps/\<guid>/env                                                                            |
| Update App's Environment Variables  | PATCH /v3/apps/\<guid>/environment_variables    
| Get App Processes by Type           | [GET /v3/apps/\<guid>/processes/\<web>](https://v3-apidocs.cloudfoundry.org/version/3.113.0/#get-a-process) 

#### [List Apps](https://v3-apidocs.cloudfoundry.org/version/3.110.0/index.html#list-apps)
**Query Parameters:** Currently supports filtering by app `names` and `space_guids` and ordering by `name`.

```bash
curl "http://localhost:9000/v3/apps?names=app1,app2&space_guids=ad0836b5-09f4-48c0-adb2-2c61e515562f,6030b015-f003-4c9f-8bb4-1ed7ae3d3659"
```

#### [Creating Apps](https://v3-apidocs.cloudfoundry.org/version/3.110.0/index.html#create-an-app)
Note : `namespace` needs to exist before creating the app.
```bash
curl "http://localhost:9000/v3/apps" \
  -X POST \
  -d '{"name":"my-app","relationships":{"space":{"data":{"guid":"<space-guid>"}}}}'
```

#### [Setting App's Current Droplet](https://v3-apidocs.cloudfoundry.org/version/3.108.0/index.html#update-a-droplet)
```bash
curl "http://localhost:9000/v3/apps/<app-guid>/relationships/current_droplet" \
  -X PATCH \
  -d '{"data":{"guid":"<droplet-guid>"}}'
```

#### [Getting App's Current Droplet](https://v3-apidocs.cloudfoundry.org/version/3.109.0/index.html#get-current-droplet)
```bash
curl "http://localhost:9000/v3/apps/<app-guid>/droplets/current"
```

#### [Scaling App's Process](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#scale-a-process)
```bash
curl "http://localhost:9000/v3/apps/<guid>/processes/<type>/actions/scale" \
  -X POST \
  -d '{ "instances": 5, "memory_in_mb": 256, "disk_in_mb": 1024 }'
```

#### [Start an app](https://v3-apidocs.cloudfoundry.org/version/3.100.0/index.html#start-an-app)
```bash
curl "http://localhost:9000/v3/apps/<app-guid>/actions/start" \
  -X POST
```

#### [Stop an app](https://v3-apidocs.cloudfoundry.org/version/3.100.0/index.html#stop-an-app)
```bash
curl "http://localhost:9000/v3/apps/<app-guid>/actions/stop" \
  -X POST
```

#### [Restart an app](https://v3-apidocs.cloudfoundry.org/version/3.110.0/index.html#restart-an-app)
```bash
curl "http://localhost:9000/v3/apps/<app-guid>/actions/restart" \
  -X POST
```

#### [Get app env](https://v3-apidocs.cloudfoundry.org/version/3.113.0/index.html#get-environment-for-an-app)
```bash
curl "http://localhost:9000/v3/apps/<app-guid>/env" 
```

#### [Update app's environment variables](https://v3-apidocs.cloudfoundry.org/version/3.113.0/index.hml#update-environment-variables-for-an-app)
```bash
curl "http://localhost:9000/v3/apps/<app-guid>/environment_variables" \
  -X PATCH \
  -d '{ "var": { "DEBUG": "false", "USER": null }'
```

### Packages

| Resource                                                                                                                | Endpoint                         |
| ----------------------------------------------------------------------------------------------------------------------- | -------------------------------- |
| Create Package                                                                                                          | POST /v3/packages                |
| List Package                                                                                                            | GET /v3/packages                 |
| Upload Package Bits                                                                                                     | POST /v3/packages/<guid>/upload  |
| [List Droplets for Package](https://v3-apidocs.cloudfoundry.org/version/3.111.0/index.html#list-droplets-for-a-package) | GET /v3/packages/<guid>/droplets |

#### [Creating Packages](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#create-a-package)
```bash
curl "http://localhost:9000/v3/packages" \
  -X POST \
  -d '{"type":"bits","relationships":{"app":{"data":{"guid":"<app-guid-goes-here>"}}}}'
```

#### [Uploading Package Bits](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#upload-package-bits)
```bash
curl "http://localhost:9000/v3/packages/<guid>/upload" \
  -X POST \
  -F bits=@"<path-to-app-source.zip>"
```

#### [List Package](https://v3-apidocs.cloudfoundry.org/version/3.111.0/index.html#list-packages)
**Query Parameters:** Currently supports filtering by `app_guids`.

```bash
curl "http://localhost:9000/v3/packages" 
```

### Builds

Docs: https://v3-apidocs.cloudfoundry.org/version/3.100.0/index.html#builds

| Resource     | Endpoint               |
| ------------ | ---------------------- |
| Get Build    | GET /v3/builds/\<guid> |
| Create Build | POST /v3/builds        |

#### [Creating Builds](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#create-a-build)
```bash
curl "http://localhost:9000/v3/builds" \
  -X POST \
  -d '{"package":{"guid":"<package-guid-goes-here>"}}'
```

### Droplet

Docs: https://v3-apidocs.cloudfoundry.org/version/3.100.0/index.html#droplets

| Resource    | Endpoint                 |
| ----------- | ------------------------ |
| Get Droplet | GET /v3/droplets/\<guid> |

### Process

Docs: https://v3-apidocs.cloudfoundry.org/version/3.100.0/index.html#processes

| Resource             | Endpoint                                 |
| -------------------- | ---------------------------------------- |
| Get Process          | GET /v3/processes/\<guid>/sidecars       |
| Get Process Sidecars | GET /v3/processes/\<guid>/sidecars       |
| Scale Process        | POST /v3/processes/\<guid>/actions/scale |
| Get Process Stats    | POST /v3/processes/\<guid>/stats         |
| List Process         | POST /v3/processes                       |

#### [Scaling Processes](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#scale-a-process)
```bash
curl "http://localhost:9000/v3/processes/<guid>/actions/scale" \
  -X POST \
  -d '{ "instances": 5, "memory_in_mb": 256, "disk_in_mb": 1024 }'
```

#### [Get Process Stats](https://v3-apidocs.cloudfoundry.org/version/3.110.0/index.html#get-stats-for-a-process)
Currently, we only support fetching stats using the process guid endpoint, i.e., POST /v3/processes/\<guid>/stats.
This endpoint supports populating only the index and state details on the response.
Support for populating other fields will come later.

#### [List Processes](https://v3-apidocs.cloudfoundry.org/version/3.111.0/index.html#list-processes)
**Query Parameters:** Currently supports filtering by `app_guids`.

### Domain

https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#domains

| Resource     | Endpoint        |
| ------------ | --------------- |
| List Domains | GET /v3/domains |

#### [List Domains](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#list-domains)
```bash
curl "http://localhost:9000/v3/domains?names=cf-apps.io" \
  -X GET
```

### Routes

| Resource                  | Endpoint                              |
| ------------------------- | ------------------------------------- |
| Get Route                 | GET /v3/routes/\<guid>                |
| Get Route List            | GET /v3/routes                        |
| Get Route Destinations    | GET /v3/routes/\<guid\>/destinations  |
| Create Route              | POST /v3/routes                       |
| Add Destinations to Route | POST /v3/routes/\<guid\>/destinations |
| Delete Route              | [DELETE /v3/routes/:guid](https://v3-apidocs.cloudfoundry.org/version/3.111.0/index.html#delete-a-route)

#### [List Routes](https://v3-apidocs.cloudfoundry.org/version/3.111.0/index.html#list-routes)
**Query Parameters:** Currently supports filtering by `app_guids`, `space_guids`, `domain_guids`, `hosts` and `paths`.

#### [Creating Routes](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#create-a-route)
```bash
curl "http://localhost:9000/v3/routes/<guid>/destinations" \
  -X POST \
  -d '{"host": "hostname","path": "/path","relationships": {"domain": {"data": { "guid": "<domain-guid-goes-here>" }},"space": {"data": { "guid": "<namespace-name>" }}}}'
```

#### [Add Destinations to Route](https://v3-apidocs.cloudfoundry.org/version/3.108.0/index.html#insert-destinations-for-a-route)
```bash
curl "http://localhost:9000/v3/routes" \
  -X POST \
  -d '{
        "destinations": [
          {
            "app": {
              "guid": "<app-guid-1>"
            }
          },
          {
            "app": {
              "guid": "<app-guid-2>",
              "process": {
                "type": "api"
              }
            },
            "port": 9000,
            "protocol": "http1"
          }
        ]
      }'
```

### Manifest

| Resource         | Endpoint                                             |
| ---------------- | ---------------------------------------------------- |
| Apply a manifest | POST /v3/spaces/\<space-guid>/actions/apply_manifest |

#### [Applying a manifest](https://v3-apidocs.cloudfoundry.org/version/3.107.0/index.html#create-a-route)
```bash
curl "http://localhost:9000/v3/spaces/<space-guid>/actions/apply_manifest" \
  -X POST \
  -H "Content-type: application/x-yaml" \
  --data-binary @<path-to-manifest.yml>
```

| Resource                           | Endpoint                                    |
| ---------------------------------- | ------------------------------------------- |
| Create a manifest diff for a space | POST /v3/spaces/\<space-guid>/manifest_diff |

#### [Create a manifest diff for a space](https://v3-apidocs.cloudfoundry.org/version/3.109.0/index.html#create-a-manifest-diff-for-a-space-experimental)
**WARNING:** Currently this endpoint always returns an empty diff.

##### Example Request
```bash
curl "http://localhost:9000/v3/spaces/<space-guid>/manifest_diff" \
  -X POST \
  -H "Content-type: application/x-yaml" \
  --data-binary @<path-to-manifest.yml>
```

##### Example Response
```json
HTTP/1.1 202 OK
Content-Type: application/json

{
  "diff": []
}
```

### Buildpacks
| Resource | Endpoint |
|--|--|
| List Buildpacks | GET /v3/buildpacks |

#### [List Buildpacks](https://v3-apidocs.cloudfoundry.org/version/3.111.0/index.html#list-buildpacks)
```bash
curl "http://localhost:9000/v3/buildpacks?order_by=postion" \
  -X GET
```

### Log-Cache API
We support basic, unauthenticated versions of the following [log-cache](https://github.com/cloudfoundry/log-cache) APIs that return hard-coded responses.

#### [Log-Cache Info](https://github.com/cloudfoundry/log-cache#get-apiv1info)

| Resource                     | Endpoint         |
| ---------------------------- | ---------------- |
| Retrieve version information | GET /api/v1/info |

#### [Log-Cache Read](https://github.com/cloudfoundry/log-cache#get-apiv1readsource-id)

| Resource                                                                                        | Endpoint                     |
| ----------------------------------------------------------------------------------------------- | ---------------------------- |
| Retrieve data by source-id. Currently returns a hard-coded empty list of Loggregator envelopes. | GET /api/v1/read/<source-id> |


### Service Instances

Docs: https://v3-apidocs.cloudfoundry.org/version/3.113.0/index.html#service-instances

| Resource                     | Endpoint         |
| ---------------------------- | ---------------- |
| Create Service Instance | POST /v3/service_instances |
| List Service Instance | GET /v3/service_instances |

#### [Create Service Instances](https://v3-apidocs.cloudfoundry.org/version/3.113.0/index.html#create-a-service-instance) 
Currently, we support creation of user-provided service instances
```bash
curl "https://localhost:9000/v3/service_instances" \
  -X POST \
  -H "Authorization: bearer [token]" \
  -H "Content-type: application/json" \
  -d '{
    "type": "user-provided",
    "name": "my_service_instance",
    "credentials": {
      "foo": "bar",
      "baz": "qux"
    },
    "tags": ["foo", "bar", "baz"],
    "syslog_drain_url": "",
    "route_service_url": "",
    "metadata": {
      "annotations": {
        "foo": "bar"
      },
      "labels": {
        "baz": "qux"
      }
    },
    "relationships": {
      "space": {
        "data": {
          "guid": "7304bc3c-7010-11ea-8840-48bf6bec2d78"
        }
      }
    }
  }'
```

#### [List Service Instances](https://v3-apidocs.cloudfoundry.org/version/3.113.0/index.html#list-service-instances)
**Query Parameters:** Currently supports filtering by service instance
`names` and `space_guids` and ordering by `name`, `created_at` or `updated_at`.
The `fields` and `per_page` parameters will be silently ignored.

### User Identity

_This is not part of the published CF API, and is not supported on CF on VMs._

It is required by the CF CLI when targetting CF-on-k8s, as the CLI needs to
know the username for display in command output and to use in creating roles
during `create-space`. This cannot be inferred from the authentication token
without the help of kubernetes since tokens might be opaque, and even when not,
kubernetes can choose non-standard fields to use as the username, and can
choose to prefix the value.

`ClientCert` authentication provides a client certificate, and we can extract
the username from the subject's common name, but we choose to deal with all
authorization header types in this endpoint to provide a useful common
abstraction.

| Resource                        | Endpoint    |
| ------------------------------- | ----------- |
| User or ServiceAccount identity | GET /whoami |
