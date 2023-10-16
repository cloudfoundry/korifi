package crds_test

import (
	"fmt"

	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tests/helpers"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gexec"
)

var _ = Describe("Using the k8s API directly", Ordered, func() {
	var (
		orgGUID               string
		orgDisplayName        string
		spaceGUID             string
		spaceDisplayName      string
		bindingName           string
		bindingUser           string
		propagatedBindingName string
		korifiAPIEndpoint     string
	)

	BeforeAll(func() {
		orgGUID = PrefixedGUID("org")
		orgDisplayName = PrefixedGUID("Org")
		spaceGUID = PrefixedGUID("space")
		spaceDisplayName = PrefixedGUID("Space")

		korifiAPIEndpoint = helpers.GetRequiredEnvVar("API_SERVER_ROOT")

		bindingName = cfUser + "-root-namespace-user"
		bindingUser = rootNamespace + ":" + cfUser

		propagatedBindingName = uuid.NewString()
	})

	AfterAll(func() {
		Expect(
			helpers.Kubectl("delete", "--ignore-not-found=true", "-n="+rootNamespace, "cforg", orgGUID),
		).To(Exit(0))
		Expect(
			helpers.Kubectl("delete", "--ignore-not-found=true", "-n="+rootNamespace, "rolebinding", bindingName),
		).To(Exit(0))
	})

	It("can create a CFOrg", func() {
		Expect(helpers.KubectlApply(`---
            apiVersion: korifi.cloudfoundry.org/v1alpha1
            kind: CFOrg
            metadata:
                namespace: %s
                name: %s
            spec:
                displayName: %s
            `, rootNamespace, orgGUID, orgDisplayName),
		).To(Exit(0))

		Expect(
			helpers.Kubectl("wait", "--for=condition=ready", "-n="+rootNamespace, "cforg/"+orgGUID, fmt.Sprintf("--timeout=%s", helpers.EventuallyTimeout())),
		).To(Exit(0))

		Expect(
			helpers.Kubectl("get", "namespace/"+orgGUID),
		).To(Exit(0))
	})

	It("can create a CFSpace", func() {
		Expect(helpers.KubectlApply(`---
            apiVersion: korifi.cloudfoundry.org/v1alpha1
            kind: CFSpace
            metadata:
                namespace: %s
                name: %s
            spec:
                displayName: %s
            `, orgGUID, spaceGUID, spaceDisplayName),
		).To(Exit(0))

		Expect(
			helpers.Kubectl("wait", "--for=condition=ready", "-n="+orgGUID, "cfspace/"+spaceGUID, fmt.Sprintf("--timeout=%s", helpers.EventuallyTimeout())),
		).To(Exit(0))

		Expect(
			helpers.Kubectl("get", "namespace/"+spaceGUID),
		).To(Exit(0))
	})

	It("can grant the necessary roles to push an app via the CLI", func() {
		Expect(
			helpers.Kubectl("create", "rolebinding", "-n="+rootNamespace, "--serviceaccount="+bindingUser, "--clusterrole=korifi-controllers-root-namespace-user", bindingName),
		).To(Exit(0))
		Expect(
			helpers.Kubectl("label", "rolebinding", bindingName, "-n="+rootNamespace, "cloudfoundry.org/role-guid="+GenerateGUID()),
		).To(Exit(0))

		Expect(
			helpers.Kubectl("create", "rolebinding", "-n="+orgGUID, "--serviceaccount="+bindingUser, "--clusterrole=korifi-controllers-organization-user", cfUser+"-org-user"),
		).To(Exit(0))
		Expect(
			helpers.Kubectl("label", "rolebinding", cfUser+"-org-user", "-n="+orgGUID, "cloudfoundry.org/role-guid="+GenerateGUID()),
		).To(Exit(0))

		Expect(
			helpers.Kubectl("create", "rolebinding", "-n="+spaceGUID, "--serviceaccount="+bindingUser, "--clusterrole=korifi-controllers-space-developer", cfUser+"-space-developer"),
		).To(Exit(0))
		Expect(
			helpers.Kubectl("label", "rolebinding", cfUser+"-space-developer", "-n="+spaceGUID, "cloudfoundry.org/role-guid="+GenerateGUID()),
		).To(Exit(0))

		Expect(helpers.Cf("api", korifiAPIEndpoint, "--skip-ssl-validation")).To(Exit(0))
		Expect(helpers.Cf("auth", cfUser)).To(Exit(0))
		Expect(helpers.Cf("target", "-o", orgDisplayName, "-s", spaceDisplayName)).To(Exit(0))

		Expect(
			helpers.Cf("push", PrefixedGUID("crds-test-app"), "-p", "../assets/dorifi", "--no-start"), // This could be any app
		).To(Exit(0))
	})

	It("can create cf-admin rolebinding which propagates to child namespaces", func() {
		Expect(helpers.KubectlApply(`---
            apiVersion: rbac.authorization.k8s.io/v1
            kind: RoleBinding
            metadata:
                annotations:
                   cloudfoundry.org/propagate-cf-role: "true"
                namespace: %s
                name: %s
            roleRef:
              apiGroup: rbac.authorization.k8s.io
              kind: ClusterRole
              name: korifi-controllers-admin
            subjects:
              - kind: ServiceAccount
                name: %s
                namespace: %s
            `, rootNamespace, propagatedBindingName, cfUser, rootNamespace),
		).To(Exit(0))

		Eventually(func(g Gomega) {
			g.Expect(helpers.Kubectl("get", "rolebinding/"+propagatedBindingName, "-n", rootNamespace)).To(Exit(0))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(helpers.Kubectl("get", "rolebinding/"+propagatedBindingName, "-n", orgGUID)).To(Exit(0))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(helpers.Kubectl("get", "rolebinding/"+propagatedBindingName, "-n", orgGUID)).To(Exit(0))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			g.Expect(helpers.Kubectl("get", "rolebinding/"+propagatedBindingName, "-n", spaceGUID)).To(Exit(0))
		}).Should(Succeed())
	})

	It("can delete the cf-admin rolebinding", func() {
		Expect(
			helpers.Kubectl("delete", "--ignore-not-found=true", "-n="+rootNamespace, "rolebinding/"+propagatedBindingName),
		).To(Exit(0))

		Expect(
			helpers.Kubectl("wait", "--for=delete", "rolebinding/"+propagatedBindingName, "-n", rootNamespace, fmt.Sprintf("--timeout=%s", helpers.EventuallyTimeout())),
		).To(Exit(0))

		Expect(
			helpers.Kubectl("wait", "--for=delete", "rolebinding/"+propagatedBindingName, "-n", orgGUID, fmt.Sprintf("--timeout=%s", helpers.EventuallyTimeout())),
		).To(Exit(0))

		Expect(
			helpers.Kubectl("wait", "--for=delete", "rolebinding/"+propagatedBindingName, "-n", spaceGUID, fmt.Sprintf("--timeout=%s", helpers.EventuallyTimeout())),
		).To(Exit(0))
	})

	It("can delete the space", func() {
		Expect(helpers.Kubectl("delete", "--ignore-not-found=true", "-n="+orgGUID, "cfspace/"+spaceGUID)).To(Exit(0))
		Expect(helpers.Kubectl("wait", "--for=delete", "namespace/"+spaceGUID)).To(Exit(0))
	})

	It("can delete the org", func() {
		Expect(helpers.Kubectl("delete", "--ignore-not-found=true", "-n="+rootNamespace, "cforgs/"+orgGUID)).To(Exit(0))

		Expect(helpers.Kubectl("wait", "--for=delete", "cforg/"+orgGUID, "-n", rootNamespace)).To(Exit(0))
		Expect(helpers.Kubectl("wait", "--for=delete", "namespace/"+orgGUID)).To(Exit(0))
	})
})
