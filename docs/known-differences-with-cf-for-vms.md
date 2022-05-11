# Known Differences between Korifi and CF-for-VMs

**This is a work in progress document and is not a comprehensive list.**

## Roles and Permissions for Orgs

### Org Manager
When interacting directly through `kubectl` a CF managed [cf_org_manager](https://github.com/cloudfoundry/korifi/blob/main/controllers/config/cf_roles/cf_org_manager.yaml) users get elevated permissions to view and list all orgs. But when listed through 
the API shim, we filter the org list to show only the orgs the user has a role-binding in. 

## Roles and Permissions for Spaces

### Org User
When interacting directly through `kubectl`, users with CF managed [cf_org_user](https://github.com/cloudfoundry/korifi/blob/main/controllers/config/cf_roles/cf_org_user.yaml) roles will have permissions to view and list all orgs and all spaces. But when listed through
the API shim, the user would only be able to list and view spaces which have role-binding corresponding to the user. 