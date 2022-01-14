# CF Resource GUID Prefixing

Date: 2022-01-14

## Status

Draft

## Context

We have received feedback from the field that naming standard k8s typed objects (e.g. namespaces, statefulsets) with raw guids makes the system difficult for a k8s operator to use.

For example:
- it's hard when looking at a list of namespaces to tell which ones are CF-created. That makes it difficult to uninstall the things you installed to a cluster if you're just kicking the tires.
- it's hard to tell which namespaces are cf spaces versus orgs. When listing other things like secrets or roles, it is hard to tell if they are in a space or an org or are totally unrelated to CF.
- it's hard to be sure which stateful sets are CF apps, versus possibly other workloads that also have GUID names.

The proposal to address this issue is to prefix the names of these resources with strings (e.g. "cf-org-") that clearly identify them as CF resources. The request is to add this prefix to the object name rather than a label or other metadata, in order to make it easily discoverable.

We considered 2 possible approaches to implement this:
1. At the time of GUID generation for spaces, orgs, and processes, we simply prepend a prefix, and then treat that prefix as an opaque part of the GUID everywhere else. This option is far less costly, but will result in GUIDs returned to the CF API client with non-standard formatting.
1. At the point(s) where we are converting a GUID stored on a CF-defined resource into a name on a non-CF-defined resource, we add the prefix to the name. When making the opposite transformation, we strip the prefix. This approach would require conversion in numerous places, given that we use namespace names throughout the code, but has the benefit that GUIDs retain GUID formattting.

## Decision
After some discussion, we decided that the first, easiest solution is the better one. We will make a small change to modfiy GUID generation for orgs, spaces, and CFProcesses. We will try to avoid writing any code that parses the GUIDs or explicitly expects the prefixes to be there. We will use the prefixes:
- "cf-org-"
- "cf-space-"
- "cf-proc-"

## Consequences

### Pros
* This solution is trivial to implement.
* This decision is reversible. So long as we don't rely on the existence of the prefix, we can change or remove it at any time.
* This decision is extensible. We can decide to apply prefixes to other object type GUIDs (e.g. CFRoute, CFApp, etc.) as we see fit.
* Because we are not introducing a discrepancy between GUID and object name, users can still rely on them to be the same. (I.e. `kubectl get secret -n $(cf space s --guid)` will continue to work)

### Cons
* Our GUIDs will no longer conform to [RFC 4122](https://www.ietf.org/rfc/rfc4122.txt), and therefore will not be convertible into binary format.

## Open Questions

* Should we extend this decision to other types, and prefix guids for things like CFRoute, CFPackage, etc?
* Some CF API clients may have fixed length columns in a database, or columns that are guid typed. Our new guids may not fit or may fail to parse. The affected clients we are aware of would need major modification to work with the API shim anyway, so we don't believe this will create a significant burden.
