package crds_test

import (
	. "code.cloudfoundry.org/korifi/controllers/controllers/workloads/testutils"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Using the k8s API directly", Ordered, func() {
	var (
		orgName        string
		orgDisplayName string
	)

	BeforeAll(func() {
		orgName = PrefixedGUID("org")
		orgDisplayName = PrefixedGUID("Org")
	})

	AfterAll(func() {
		expectCommandToSucceed(kubectl("delete", "--ignore-not-found=true", "-n="+rootNamespace, "cforg/"+orgName))
	})

	It("creates a CFOrg (and its namespace)", func() {
		kubectlApply := kubectl("apply", "-f=-")
		writeToStdIn(kubectlApply, `---
            apiVersion: korifi.cloudfoundry.org/v1alpha1
            kind: CFOrg
            metadata:
                namespace: %s
                name: %s
            spec:
                displayName: %s
        `,
			rootNamespace, orgName, orgDisplayName,
		)
		expectCommandToSucceed(kubectlApply)

		expectCommandToSucceed(kubectl("wait", "--for=condition=ready", "-n="+rootNamespace, "cforg/"+orgName))
		expectCommandToSucceed(kubectl("get", "namespace/"+orgName))
	})
})
