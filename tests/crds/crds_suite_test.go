package crds_test

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func TestCrds(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CRDs Suite")
}

var rootNamespace string

var _ = BeforeSuite(func() {
	var ok bool
	rootNamespace, ok = os.LookupEnv("ROOT_NAMESPACE")
	if !ok {
		rootNamespace = "cf"
	}

	expectCommandToSucceed(kubectl("get", "namespace/"+rootNamespace),
		"Could not find root namespace called %q", rootNamespace)
})

func kubectl(words ...string) *exec.Cmd {
	return exec.Command("kubectl", words...)
}

func writeToStdIn(cmd *exec.Cmd, stdinText string, sprintfArgs ...any) {
	stdin, err := cmd.StdinPipe()
	Expect(err).NotTo(HaveOccurred())
	_, err = stdin.Write([]byte(fmt.Sprintf(stdinText, sprintfArgs...)))
	Expect(err).NotTo(HaveOccurred())
	stdin.Close()
}

func expectCommandToSucceed(cmd *exec.Cmd, optionalDescription ...any) {
	session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session, "10s").Should(gexec.Exit(0), optionalDescription...)
}
