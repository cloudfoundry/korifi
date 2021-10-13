# Strategy for Testing Frameworks


Date: 2021-08-27

## Status

Accepted

## Context

Now that we are working on the cf-on-k8s project in earnest, we have the opportunity with a clean slate to decide on testing frameworks for our projects.

After investigating other Kubernetes projects, we landed on [ginkgo](https://github.com/onsi/ginkgo) and [spec](https://github.com/sclevine/spec) as potential testing framework candidates.

For matchers, we decided to stick with the standard [gomega](https://github.com/onsi/gomega).

### Ginkgo

https://github.com/onsi/ginkgo

Ginkgo uses Go's testing package and can live alongside your existing testing tests. Ginkgo allows you to write tests in Go using expressive [Behavior-Driven Development](https://en.wikipedia.org/wiki/Behavior-driven_development) ("BDD") style.

* Pro: We know this one well and it supports the BDD testing style we’re familiar with from Rspec
* Con: It doesn’t support Go 1.7+ subtests, so the output isn’t very friendly when run via `go test` instead of the “ginkgo” command. This mostly affects IDE integrations, and fixed for Goland with https://github.com/matt-royal/biloba

### Spec
https://github.com/sclevine/spec

Spec is a simple BDD test organizer for Go. It minimally extends the standard library testing package by facilitating easy organization of Go 1.7+ [subtests](https://go.dev/blog/subtests).

* Pro: Goes to complex lengths internally to avoid test pollution, which can be a tricky problem in ginkgo.
* Pro: Stephen Levine works for VMware on the buildpacks team and has historically been very helpful when we’ve had questions
* Con: the documentation is lacking

Comparing the two, we had the following questions and concerns:
* **Who are our contributors?**
  * Existing CF users/contributors would be most likely to contribute initially
  * Eventually members of the wider Kubernetes community
* **Concern about Go 1.7+ subtest support in Ginkgo**
  * Mostly affects IDE though
* **Ginkgo can be a blocker to community engagement as a library not as commonly used in the Go community**
* **Not many complaints against BDD itself (e.g. “when x,y,z, it does…”)**
  * There may be some traction with the sclevine/spec library in the buildpacks community

## Decision

The cf-k8s-controller project will use spec as its testing framework and gomega for matching.


## Consequences
* Kubebuilder generates ginkgo tests by default, there will be inital overhead to convert them to spec.

