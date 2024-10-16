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

## Container Lifecycle

### Rolling Updates
In Kofiri `--strategy=rolling` is implemented using k8S rolling update capabilities of the scheduler. At the moment korifi uses statefulsets to run the app workloads. Rolling update for statefulsets stops the old instance before starting the new one, for ordering reasons. If the app has only one instance the udpate will cause a downtime. Apps with more than one instance won't experience any downtime, but they will have one instance less up and running during the update.

### Stack Changes
While in CF for VMs the staging process yields a droplet, which is a stripped container image without base layer/operating system.
In Korifi a fully fledged image is created which includes the base operating system(stack). 
This alters the behaviour in case of stack updates. CF for VMs does purposefully combine stack and droplet at container instance creation time - thus a restart of an app consumes a new stack. In Korifi only a restage of an app consumes the new stack. One might need to detect that and dynamically rebase a droplet onto a new stack in korifi before starting the container to yield the same behaviour here. 

## Apps
### App Security Groups

CF supports [app security groups](https://docs.cloudfoundry.org/concepts/asg.html) which could be used to controll the egress traffic.

### Instance Identity Credentials

CF manages for every app instance unique certificates which are known as [instance identity credentials](https://docs.cloudfoundry.org/devguide/deploy-apps/instance-identity.html). They are used e.g. by the GoRouter to make sure that an incomming request reaches the right app instance.

### SSH Access

The CF CLI supports [ssh log in](https://docs.cloudfoundry.org/devguide/deploy-apps/ssh-apps.html) to running CF app instances.

### Setting app current droplet

When the app current droplet is set, this causes statefulset pod restart, effectively picking up the new droplet immediately (see https://github.com/cloudfoundry/korifi/issues/3234 for details)
