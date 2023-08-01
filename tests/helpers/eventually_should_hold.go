package helpers

import (
	. "github.com/onsi/gomega" //lint:ignore ST1001 this is a test file
)

func EventuallyShouldHold(condition func(g Gomega)) {
	Eventually(condition).Should(Succeed())
	Consistently(condition).Should(Succeed())
}
