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
			"10s",
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
			"10s",
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
			"10s",
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
			"10s",
		).Should(Exit(0))
	})
})
