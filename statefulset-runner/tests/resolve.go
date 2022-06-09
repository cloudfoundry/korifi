package tests

import (
	"fmt"
	"net"

	. "github.com/onsi/gomega"
)

func RetryResolveHost(host, failureMessage string) {
	Eventually(func() error {
		_, err := net.LookupIP(host)

		return err
	}, "30s").Should(Succeed(), fmt.Sprintf("Could not resolve host %q: %s", host, failureMessage))
}
