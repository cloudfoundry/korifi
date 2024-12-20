package helpers

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
	"github.com/onsi/gomega/gexec"
)

func GenerateWebhookManifest(path string) string {
	tmpDir, err := os.MkdirTemp("", "")
	Expect(err).NotTo(HaveOccurred())

	controllerGenSession, err := gexec.Start(exec.Command(
		"controller-gen",
		"paths="+path,
		"webhook",
		fmt.Sprintf("output:webhook:artifacts:config=%s", tmpDir),
	), GinkgoWriter, GinkgoWriter)

	Expect(err).NotTo(HaveOccurred())
	Eventually(controllerGenSession).Should(gexec.Exit(0))

	return filepath.Join(tmpDir, "manifests.yaml")
}
