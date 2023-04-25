package helpers

import (
	. "github.com/onsi/gomega"
)

func EventuallyShouldHold(condition func(g Gomega)) {
	Eventually(condition).Should(Succeed())
	Consistently(condition).Should(Succeed())
}
