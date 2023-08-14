package helpers

import (
	"os"
	"strconv"
	"time"

	. "github.com/onsi/gomega" //lint:ignore ST1001 this is a test file
)

func EventuallyShouldHold(condition func(g Gomega)) {
	Eventually(condition).WithTimeout(EventuallyTimeout()).Should(Succeed())
	Consistently(condition).Should(Succeed())
}

func EventuallyTimeout() time.Duration {
	eventuallyTimeout := 4 * time.Minute
	eventuallyTimeoutSecondsString := os.Getenv("E2E_EVENTUALLY_TIMEOUT_SECONDS")

	if eventuallyTimeoutSecondsString != "" {
		eventuallyTimeoutSeconds, err := strconv.Atoi(eventuallyTimeoutSecondsString)
		Expect(err).NotTo(HaveOccurred())
		eventuallyTimeout = time.Duration(eventuallyTimeoutSeconds) * time.Second
	}

	return eventuallyTimeout
}
