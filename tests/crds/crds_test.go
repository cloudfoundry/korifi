package crds_test

import (
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"github.com/cloudfoundry/cf-test-helpers/cf"
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
		testCLIUser           string
		cfUserRoleBindingName string
		korifiAPIEndpoint     string
		skipSSL               string
	)

	BeforeAll(func() {
		orgGUID = PrefixedGUID("org")
		orgDisplayName = PrefixedGUID("Org")
		spaceGUID = PrefixedGUID("space")
		spaceDisplayName = PrefixedGUID("Space")

		testCLIUser = GetRequiredEnvVar("CRDS_TEST_CLI_USER")
		korifiAPIEndpoint = GetRequiredEnvVar("CRDS_TEST_API_ENDPOINT")
		skipSSL = GetDefaultedEnvVar("CRDS_TEST_SKIP_SSL", "false")

		cfUserRoleBindingName = testCLIUser + "-root-namespace-user"
	})

	AfterAll(func() {
		Eventually(
			kubectl("delete", "--ignore-not-found=true", "-n="+rootNamespace, "cforg", orgGUID),
			"20s",
		).Should(Exit(0))

		Eventually(
			kubectl("delete", "--ignore-not-found=true", "-n="+rootNamespace, "rolebinding", cfUserRoleBindingName),
		).Should(Exit(0))
	})

	It("can create a CFOrg", func() {
		applyCFOrg := kubectlApply(`---
            apiVersion: korifi.cloudfoundry.org/v1alpha1
            kind: CFOrg
            metadata:
                namespace: %s
                name: %s
            spec:
                displayName: %s
            `, rootNamespace, orgGUID, orgDisplayName)
		Eventually(applyCFOrg).Should(Exit(0))

		Eventually(
			kubectl("wait", "--for=condition=ready", "-n="+rootNamespace, "cforg/"+orgGUID),
			"20s",
		).Should(Exit(0))

		Eventually(
			kubectl("get", "namespace/"+orgGUID),
		).Should(Exit(0))
	})

	It("can create a CFSpace", func() {
		applyCFSpace := kubectlApply(`---
            apiVersion: korifi.cloudfoundry.org/v1alpha1
            kind: CFSpace
            metadata:
                namespace: %s
                name: %s
            spec:
                displayName: %s
            `, orgGUID, spaceGUID, spaceDisplayName)
		Eventually(applyCFSpace).Should(Exit(0))

		Eventually(
			kubectl("wait", "--for=condition=ready", "-n="+orgGUID, "cfspace/"+spaceGUID),
			"20s",
		).Should(Exit(0))

		Eventually(
			kubectl("get", "namespace/"+spaceGUID),
		).Should(Exit(0))
	})

	It("can grant the necessary roles to push an app via the CLI", func() {
		Eventually(
			kubectl("create", "rolebinding", "-n="+rootNamespace, "--user="+testCLIUser, "--clusterrole=korifi-controllers-root-namespace-user", cfUserRoleBindingName),
		).Should(Exit(0))

		Eventually(
			kubectl("create", "rolebinding", "-n="+orgGUID, "--user="+testCLIUser, "--clusterrole=korifi-controllers-organization-user", testCLIUser+"-org-user"),
		).Should(Exit(0))

		Eventually(
			kubectl("create", "rolebinding", "-n="+spaceGUID, "--user="+testCLIUser, "--clusterrole=korifi-controllers-space-developer", testCLIUser+"-space-developer"),
		).Should(Exit(0))

		loginAs(korifiAPIEndpoint, skipSSL == "true", testCLIUser)

		Eventually(cf.Cf("target", "-o", orgDisplayName, "-s", spaceDisplayName)).Should(Exit(0))

		Eventually(
			cf.Cf("push", PrefixedGUID("crds-test-app"), "-p", "../smoke/assets/test-node-app", "--no-start"), // This could be any app
			"20s",
		).Should(Exit(0))
	})

	It("can create cf-admin rolebinding which propagates to child namespaces", func() {
		applyCFAdminRoleBinding := kubectlApply(`---
            apiVersion: rbac.authorization.k8s.io/v1
            kind: RoleBinding
            metadata:
                annotations:
                   cloudfoundry.org/propagate-cf-role: "true"
                namespace: %s
                name: cf-admin-test-cli-role-binding
            roleRef:
              apiGroup: rbac.authorization.k8s.io
              kind: ClusterRole
              name: korifi-controllers-admin
            subjects:
              - kind: User
                name: %s
            `, rootNamespace, testCLIUser)
		Eventually(applyCFAdminRoleBinding).Should(Exit(0))

		// In case of gexec.Exit(), eventually doesn't block for "n" seconds
		// but will return (and fail) as soon as the mismatched exit code arrives,
		// To get around it, we wrapped it in an outer eventually block
		// See: https://onsi.github.io/gomega/#aborting-eventuallyconsistently

		Eventually(func(g Gomega) {
			Eventually(kubectl("get", "rolebinding/cf-admin-test-cli-role-binding", "-n", rootNamespace)).Should(Exit(0))
		}, "20s").Should(Succeed())

		Eventually(func(g Gomega) {
			Eventually(kubectl("get", "rolebinding/cf-admin-test-cli-role-binding", "-n", orgGUID)).Should(Exit(0))
		}, "20s").Should(Succeed())

		Eventually(func(g Gomega) {
			Eventually(kubectl("get", "rolebinding/cf-admin-test-cli-role-binding", "-n", spaceGUID)).Should(Exit(0))
		}, "20s").Should(Succeed())
	})

	It("can delete the cf-admin rolebinding", func() {
		Eventually(
			kubectl("delete", "--ignore-not-found=true", "-n="+rootNamespace, "rolebinding/cf-admin-test-cli-role-binding"),
			"20s",
		).Should(Exit(0))

		Eventually(kubectl("wait", "--for=delete", "rolebinding/cf-admin-test-cli-role-binding", "-n", rootNamespace, "--timeout=60s"), "60s").Should(Exit(0))

		Eventually(kubectl("wait", "--for=delete", "rolebinding/cf-admin-test-cli-role-binding", "-n", orgGUID, "--timeout=60s"), "60s").Should(Exit(0))

		Eventually(kubectl("wait", "--for=delete", "rolebinding/cf-admin-test-cli-role-binding", "-n", spaceGUID, "--timeout=60s"), "60s").Should(Exit(0))

	})

	It("can delete the org", func() {
		Eventually(
			kubectl("delete", "--ignore-not-found=true", "-n="+rootNamespace, "cforgs/"+orgGUID),
			"20s",
		).Should(Exit(0))

		Eventually(kubectl("wait", "--for=delete", "cforg/"+orgGUID, "-n", rootNamespace, "--timeout=20s"), "20s").Should(Exit(0))

		Eventually(kubectl("wait", "--for=delete", "namespace/"+orgGUID, "--timeout=60s"), "60s").Should(Exit(0))

		Eventually(kubectl("wait", "--for=delete", "namespace/"+spaceGUID, "--timeout=60s"), "60s").Should(Exit(0))

	})
})
