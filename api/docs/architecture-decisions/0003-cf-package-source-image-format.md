# 3. CF Package Source Image Format Decision

Date: 2021-09-20

## Status

Accepted

## Context

As part of [issue 35](https://github.com/cloudfoundry/cf-k8s-api/issues/35) we explored the feasibility of using `imgpkg` images instead of single-layer source images in our Staging flow. 

### CF for VMs context
During `cf push`, the CLI will package up the local app source code into a zip file (referred to as app bits) and send it to the Cloud Controller API. The Cloud Controller API will do some additional processing (a big portion of this is a feature called resource matching) and send the app `bits` to the packages bucket in the `blobstore`. The associated CF Package for these bits will be updated to have the state `READY` to indicate that the source code has been fully uploaded.

### CF for K8s context
In CF for K8s we use [`kpack`](https://github.com/pivotal/kpack) for application staging and provide the app source code as an OCI image. In this flow, the CLI will still package the local app source code as a zip file and sent it to the Cloud Controller API. Instead of sending it to a blobstore, though, Cloud Controller will pass the zip off to another component called [`registry-buddy`](https://github.com/cloudfoundry/capi-k8s-release/tree/main/src/registry-buddy) which [converts the zip file into a single-layer OCI image](https://github.com/cloudfoundry/capi-k8s-release/blob/main/src/registry-buddy/package_upload/package_upload.go) that contains the app source. It then pushes that image to an image registry. Once this is done the associated CF Package is updated to have the state `READY` and Cloud Controller implicitly knows where the image was pushed to based on the configured image registry / package guid.

Some of the questions we intend to answer:

**Feasibility** - We plan on using kpack for application staging in CF on K8s. Will moving to a imgpkg format break the feasibility of staging with kpack?

- We determined that kpack does support `imgpkg` built images.

**Complexity** - Do we take up additional complexity to convert the source code to a `imgpkg` and to stage with kpack. How will re-staging work when there are stack updates?

- In order to build `imgpkg` images, you need to add an additional `.imgpkg/images.yaml` file to the source code and provide it to the `imgpkg` cli
  - Currently, no official support for imgpkg client. However, some users have used classes from [pkg/imgpkg/cmd](https://github.com/vmware-tanzu/carvel-imgpkg/tree/develop/pkg/imgpkg/cmd) repository. The repo doesn't maintain backwards compatibility. 
- Restaging should not affect `imgpkg` images differently from single-layer OCI images.

**Benefits** - Do we see benefits of using imgpkg be of value to CF users.

- It is possible that the bundles concept from `imgpkg` may lend itself to supporting resource matching in CF, however a deeper investigation/spike should be performed, as that feature is not part of the cf-on-k8s MVP
- There is some small gain for standardizing on source image format with the Carvel toolchain.


## Decision

At this point, we do not recommend switching from single-layer OCI images to `imgpkg` images. The potential upside is marginal, the documentation is sparse, and the cost to investigate and implement is high.

## Consequences

- We do not follow the standard for source images set by the Carvel toolchain
  - However, the source packaging implementation can be easily modified later. There will be a clear divide in the Shim package upload code to support future extension.

