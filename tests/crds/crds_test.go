package crds_test

import (
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"
	"code.cloudfoundry.org/korifi/tests/helpers"
	"github.com/cloudfoundry/cf-test-helpers/cf"
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
		skipSSL               string
	)

	BeforeAll(func() {
		orgGUID = PrefixedGUID("org")
		orgDisplayName = PrefixedGUID("Org")
		spaceGUID = PrefixedGUID("space")
		spaceDisplayName = PrefixedGUID("Space")

		korifiAPIEndpoint = helpers.GetRequiredEnvVar("CRDS_TEST_API_ENDPOINT")
		skipSSL = helpers.GetDefaultedEnvVar("CRDS_TEST_SKIP_SSL", "false")

		bindingName = cfUser + "-root-namespace-user"
		bindingUser = rootNamespace + ":" + cfUser

		propagatedBindingName = uuid.NewString()
	})

	AfterAll(func() {
		deleteOrg := kubectl("delete", "--ignore-not-found=true", "-n="+rootNamespace, "cforg", orgGUID)
		deleteRoleBinding := kubectl("delete", "--ignore-not-found=true", "-n="+rootNamespace, "rolebinding", bindingName)

		Eventually(deleteOrg, "20s").Should(Exit(0), "deleteOrg")
		Eventually(deleteRoleBinding, "20s").Should(Exit(0), "deleteRoleBinging")
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
			kubectl("create", "rolebinding", "-n="+rootNamespace, "--serviceaccount="+bindingUser, "--clusterrole=korifi-controllers-root-namespace-user", bindingName),
		).Should(Exit(0))
		Eventually(
			kubectl("label", "rolebinding", bindingName, "-n="+rootNamespace, "cloudfoundry.org/role-guid="+GenerateGUID()),
		).Should(Exit(0))

		Eventually(
			kubectl("create", "rolebinding", "-n="+orgGUID, "--serviceaccount="+bindingUser, "--clusterrole=korifi-controllers-organization-user", cfUser+"-org-user"),
		).Should(Exit(0))
		Eventually(
			kubectl("label", "rolebinding", cfUser+"-org-user", "-n="+orgGUID, "cloudfoundry.org/role-guid="+GenerateGUID()),
		).Should(Exit(0))

		Eventually(
			kubectl("create", "rolebinding", "-n="+spaceGUID, "--serviceaccount="+bindingUser, "--clusterrole=korifi-controllers-space-developer", cfUser+"-space-developer"),
		).Should(Exit(0))
		Eventually(
			kubectl("label", "rolebinding", cfUser+"-space-developer", "-n="+spaceGUID, "cloudfoundry.org/role-guid="+GenerateGUID()),
		).Should(Exit(0))

		loginAs(korifiAPIEndpoint, skipSSL == "true", cfUser)

		Eventually(cf.Cf("target", "-o", orgDisplayName, "-s", spaceDisplayName)).Should(Exit(0))

		Eventually(
			cf.Cf("push", PrefixedGUID("crds-test-app"), "-p", "../assets/dorifi", "--no-start"), // This could be any app
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
                name: %s
            roleRef:
              apiGroup: rbac.authorization.k8s.io
              kind: ClusterRole
              name: korifi-controllers-admin
            subjects:
              - kind: ServiceAccount
                name: %s
                namespace: %s
            `, rootNamespace, propagatedBindingName, cfUser, rootNamespace)
		Eventually(applyCFAdminRoleBinding).Should(Exit(0))

		Eventually(func() int {
			return kubectl("get", "rolebinding/"+propagatedBindingName, "-n", rootNamespace).Wait().ExitCode()
		}, "20s").Should(BeNumerically("==", 0))

		Eventually(func() int {
			return kubectl("get", "rolebinding/"+propagatedBindingName, "-n", orgGUID).Wait().ExitCode()
		}, "20s").Should(BeNumerically("==", 0))

		Eventually(func() int {
			return kubectl("get", "rolebinding/"+propagatedBindingName, "-n", spaceGUID).Wait().ExitCode()
		}, "20s").Should(BeNumerically("==", 0))
	})

	It("can delete the cf-admin rolebinding", func() {
		Eventually(
			kubectl("delete", "--ignore-not-found=true", "-n="+rootNamespace, "rolebinding/"+propagatedBindingName),
			"20s",
		).Should(Exit(0))

		Eventually(kubectl("wait", "--for=delete", "rolebinding/"+propagatedBindingName, "-n", rootNamespace, "--timeout=60s"), "60s").Should(Exit(0))

		Eventually(kubectl("wait", "--for=delete", "rolebinding/"+propagatedBindingName, "-n", orgGUID, "--timeout=60s"), "60s").Should(Exit(0))

		Eventually(kubectl("wait", "--for=delete", "rolebinding/"+propagatedBindingName, "-n", spaceGUID, "--timeout=60s"), "60s").Should(Exit(0))
	})

	It("can delete the space", func() {
		Eventually(kubectl("delete", "--ignore-not-found=true", "-n="+orgGUID, "cfspace/"+spaceGUID), "120s").Should(Exit(0))
		Eventually(kubectl("wait", "--for=delete", "namespace/"+spaceGUID)).Should(Exit(0))
	})

	It("can delete the org", func() {
		Eventually(kubectl("delete", "--ignore-not-found=true", "-n="+rootNamespace, "cforgs/"+orgGUID), "120s").Should(Exit(0))

		Eventually(kubectl("wait", "--for=delete", "cforg/"+orgGUID, "-n", rootNamespace)).Should(Exit(0))
		Eventually(kubectl("wait", "--for=delete", "namespace/"+orgGUID)).Should(Exit(0))
	})
})
