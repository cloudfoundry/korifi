# 4. Strategy for Continuous Integration

Date: 2021-08-27

## Status

Accepted

## Context
Now that we are working on the cf-on-k8s project in earnest, we have the opportunity with a clean slate to decide on continuous integration tools for our projects.

The options considered below are pulled from popular options for CI in Kubernetes. The particular ordering reflects the priority/consideration level with the highest value first. This is to strike a balance between consistency with other CFF projects and what other projects in the K8s space are using.

| Tool  | OSS  | Web UI / CLI | Hosted | Auto Scalable  |  DSL |  Dependencies / Incorporates |  Users |  Other |
| ------------ | ------------ | ------------ | ------------ | ------------ | ------------ | ------------ | ------------ | ------------ |
|  [Concourse](concourse-ci.org "Concourse") | Yes  | Both  | Self  | No  | YAML  |   | CFF Community  |   |
|  [Tekton](tekton.dev "Tekton") | Yes  | Both  | Applied in K8s cluster  | Depends on cluster  | K8s CRD  |   | K8s community MAPBU teams  |  “Batteries included” in Kontinue |
|  [CircleCI](circleci.com "CircleCI") | OSS option  | Both  | Yes/Self  | Depends  | YAML Somewhat Complex  |   | JS projects Azure container Service Engine  |   |
|  [GitHub Actions](https://github.com/features/actions "GitHub Actions") |  Yes | Both  | In Cluster  | Depends on cluster  | YAML simple  | Github  | Kubebuilder SAP  | Integrated in the repo  |
|  [JenkinsX](https://jenkins-x.io/ "JenkinsX") | Yes  | Both  | In Cluster  | Depends on cluster  | YAML  | | Terraform, Helm, Kustomize, GitOps, Tekton  |   |
|  [Jenkins](https://www.jenkins.io/ "Jenkins") | Yes  | Web UI/API  | Self/In cluster  | Possible depending on configuration  | Groovy-ish Complex  | Java  | Ubiquitous  |  Requires lots of configuration and management |
| [DroneCI](drone.io "DroneCI")  |  Yes | Both  | Self  | Yes  | YAML, Mixture of Concourse and K8s CRD  |   |   | 1st class support for various use cases: docker, K8s, local, etc, Extensible
  |

### What is our CI trying to accomplish?
* Run unit tests
* Integration tests on various clusters/IaaSes
  * Possibly involving provisioning
* Cut releases

For now, Github Actions is nice to keep everything together and publicly accessible. This ties into using Github Projects as well.

---

## Decision
Start with Github Actions for CI, this might cover all we need, but can reevaluate again later. When commercial options are needed, use Runway CI.


## Consequences
* We can take advantage of close integration with Github Projects, to keep everything centralized in Github. We hope that this will make it easier for contributors to onboard and find everything as we ramp up.
