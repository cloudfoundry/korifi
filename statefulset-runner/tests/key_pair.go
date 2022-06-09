package tests

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"code.cloudfoundry.org/tlsconfig/certtest"
	. "github.com/onsi/gomega"
)

func GenerateKeyPair(name string) (string, string) {
	dir, _ := GenerateKeyPairDir(name, name)

	return path.Join(dir, fmt.Sprintf("%s.crt", name)), path.Join(dir, fmt.Sprintf("%s.key", name))
}

// GenerateKeyPairDir generates a key pair in a temporary folder, returning
// its path and the authority PEM. The two files will be called <name>.crt and
// <name>.key
func GenerateKeyPairDir(name, domain string) (string, []byte) {
	authority, err := certtest.BuildCA(name)
	Expect(err).NotTo(HaveOccurred())

	cert, err := authority.BuildSignedCertificate(name, certtest.WithDomains(domain))
	Expect(err).NotTo(HaveOccurred())

	certData, keyData, err := cert.CertificatePEMAndPrivateKey()
	Expect(err).NotTo(HaveOccurred())

	tmpDir, err := ioutil.TempDir("", "")
	Expect(err).ToNot(HaveOccurred())
	err = ioutil.WriteFile(path.Join(tmpDir, fmt.Sprintf("%s.crt", name)), certData, os.ModePerm)
	Expect(err).ToNot(HaveOccurred())
	err = ioutil.WriteFile(path.Join(tmpDir, fmt.Sprintf("%s.key", name)), keyData, os.ModePerm)
	Expect(err).ToNot(HaveOccurred())

	caBytes, err := authority.CertificatePEM()
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(path.Join(tmpDir, fmt.Sprintf("%s.ca", name)), caBytes, os.ModePerm)
	Expect(err).ToNot(HaveOccurred())

	return tmpDir, caBytes
}
