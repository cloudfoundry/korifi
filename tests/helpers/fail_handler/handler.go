package fail_handler

import (
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	rest "k8s.io/client-go/rest"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

var Always types.GomegaMatcher = gomega.ContainSubstring("")

func New(name string, hooks map[types.GomegaMatcher]func(config *rest.Config)) func(message string, callerSkip ...int) {
	config, err := controllerruntime.GetConfig()
	if err != nil {
		panic(err)
	}

	return func(message string, callerSkip ...int) {
		fmt.Fprintf(ginkgo.GinkgoWriter, "Fail Handler %s\n", name)

		if len(callerSkip) > 0 {
			callerSkip[0] = callerSkip[0] + 2
		} else {
			callerSkip = []int{2}
		}

		defer func() {
			fmt.Fprintf(ginkgo.GinkgoWriter, "Fail Handler %s: completed\n", name)
			ginkgo.Fail(message, callerSkip...)
		}()

		for matcher, hook := range hooks {
			matchingMessage, err := matcher.Match(message)
			if err != nil {
				fmt.Fprintf(ginkgo.GinkgoWriter, "Failed to match message: %v\n", err)
				return
			}

			if matchingMessage {
				hook(config)
			}
		}
	}
}
