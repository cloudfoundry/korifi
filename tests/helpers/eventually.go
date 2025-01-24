package helpers

import (
	"os"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2" //lint:ignore ST1001 this is a test file
	. "github.com/onsi/gomega"    //lint:ignore ST1001 this is a test file
)

func EventuallyShouldHold(condition func(g Gomega)) {
	GinkgoHelper()

	Eventually(condition).Should(Succeed())
	Consistently(condition).Should(Succeed())
}

func EventuallyTimeout() time.Duration {
	GinkgoHelper()

	return getDuration("E2E_EVENTUALLY_TIMEOUT_SECONDS", 4*60)
}

func EventuallyPollingInterval() time.Duration {
	GinkgoHelper()

	return getDuration("E2E_EVENTUALLY_POLLING_INTERVAL_SECONDS", 2)
}

func getDuration(envName string, defaultDurationSeconds int) time.Duration {
	GinkgoHelper()

	durationSecondsString := os.Getenv(envName)

	if durationSecondsString != "" {
		durationSeconds, err := strconv.Atoi(durationSecondsString)
		Expect(err).NotTo(HaveOccurred())
		return time.Duration(durationSeconds) * time.Second
	}

	return time.Duration(defaultDurationSeconds) * time.Second
}
