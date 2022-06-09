package integration

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"gopkg.in/yaml.v2"

	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/tests"
)

type EiriniBinaries struct {
	EiriniController Binary `json:"eirini_controller"`
	ExternalBinsPath bool
	BinsPath         string
	CertsPath        string
	CABundle         []byte
}

func NewEiriniBinaries() EiriniBinaries {
	bins := EiriniBinaries{}

	telepresenceService := "something-random"
	telepresenceDomain := fmt.Sprintf("%s.default.svc", telepresenceService)

	bins.CertsPath, bins.CABundle = tests.GenerateKeyPairDir("tls", telepresenceDomain)

	bins.setBinsPath()
	bins.EiriniController = NewBinary("code.cloudfoundry.org/korifi/statefulset-runner/cmd", bins.BinsPath, bins.CertsPath)

	return bins
}

func (b *EiriniBinaries) TearDown() {
	gexec.CleanupBuildArtifacts()

	if !b.ExternalBinsPath {
		os.RemoveAll(b.BinsPath)
	}

	os.RemoveAll(b.CertsPath)
}

func (b *EiriniBinaries) setBinsPath() {
	binsPath := os.Getenv("EIRINI_BINS_PATH")
	b.ExternalBinsPath = true

	if binsPath == "" {
		b.ExternalBinsPath = false

		var err error
		binsPath, err = ioutil.TempDir("", "bins")
		Expect(err).NotTo(HaveOccurred())
	}

	b.BinsPath = binsPath
}

type Binary struct {
	PackagePath string `json:"src_path"`
	BinPath     string `json:"bin_path"`
	LocksDir    string `json:"locks_dir"`
	CertsPath   string `json:"cert_path"`
}

func NewBinary(packagePath, binsPath string, certsPath string) Binary {
	paths := strings.Split(packagePath, "/")
	binName := paths[len(paths)-1]

	return Binary{
		PackagePath: packagePath,
		BinPath:     filepath.Join(binsPath, binName),
		LocksDir:    filepath.Join(binsPath, ".locks"),
		CertsPath:   certsPath,
	}
}

func (b *Binary) Run(config interface{}, envVars ...string) (*gexec.Session, string) {
	configBytes, err := yaml.Marshal(config)
	Expect(err).NotTo(HaveOccurred())

	var configFile string
	if config != nil {
		configFile = tests.WriteTempFile(configBytes, filepath.Base(b.BinPath)+"-config.yaml")
	}

	env := []string{
		fmt.Sprintf("%s=%s", eirinictrl.EnvEiriniCertsDir, b.CertsPath),
	}
	env = append(env, envVars...)

	return b.RunWithConfig(configFile, env...), configFile
}

func (b *Binary) RunWithConfig(configFilePath string, envVars ...string) *gexec.Session {
	b.buildIfNecessary()

	var args []string
	if configFilePath != "" {
		args = append(args, "-c", configFilePath)
	}

	command := exec.Command(b.BinPath, args...) //#nosec G204
	command.Env = envVars
	session, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred())

	return session
}

func (b *Binary) Restart(configFilePath string, runningSession *gexec.Session) *gexec.Session {
	envVars := runningSession.Command.Env
	runningSession.Kill().Wait()

	return b.RunWithConfig(configFilePath, envVars...)
}

// Build builds the binary. Normally, you should not use this function as it is
// built if needed upon first run anyway. However, sometimes it might make sense
// to explicitly build a common binary that is used across all the tests
// in SynchronizedBeforeSuite thus preventing running the build on concurrent nodes.
//
// For example, EATs tests will always run API, therefore it is a good idea to
// build it in advance.
func (b *Binary) Build() {
	b.buildIfNecessary()
}

func (b *Binary) buildIfNecessary() {
	if _, err := os.Stat(b.BinPath); err == nil {
		return
	}

	lock := flock.New(b.BinPath + ".lock")
	err := lock.Lock()
	Expect(err).NotTo(HaveOccurred())

	defer func() {
		Expect(lock.Unlock()).To(Succeed())
	}()

	_, err = os.Stat(b.BinPath)
	if os.IsNotExist(err) {
		b.build()
	}
}

func (b *Binary) build() {
	compiledPath, err := gexec.Build(b.PackagePath)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.MkdirAll(filepath.Dir(b.BinPath), 0o755)).To(Succeed())

	Expect(os.Link(compiledPath, b.BinPath)).To(Succeed())
}
