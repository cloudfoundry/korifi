package fail_handler

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	rest "k8s.io/client-go/rest"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

var Always types.GomegaMatcher = gomega.ContainSubstring("")

type Hook struct {
	Matcher types.GomegaMatcher
	Hook    func(config *rest.Config, failure TestFailure)
}

type TestFailure struct {
	StartTime time.Time
	Message   string
}

type Handler struct {
	startTime time.Time
	name      string
	hooks     []Hook
}

func (h *Handler) RegisterFailHandler(hook Hook) {
	h.hooks = append(h.hooks, hook)
}

func (h *Handler) Fail(message string, callerSkip ...int) {
	fmt.Fprintf(ginkgo.GinkgoWriter, "Fail Handler %s\n", h.name)

	if len(callerSkip) > 0 {
		callerSkip[0] = callerSkip[0] + 2
	} else {
		callerSkip = []int{2}
	}

	defer func() {
		fmt.Fprintf(ginkgo.GinkgoWriter, "Fail Handler %s: completed\n", h.name)
		ginkgo.Fail(message, callerSkip...)
	}()

	config, err := controllerruntime.GetConfig()
	if err != nil {
		fmt.Fprintf(ginkgo.GinkgoWriter, "Fail to get controller config: %v\n", err)
		return
	}

	for _, hook := range h.hooks {
		matchingMessage, err := hook.Matcher.Match(message)
		if err != nil {
			fmt.Fprintf(ginkgo.GinkgoWriter, "Failed to match message: %v\n", err)
			return
		}

		if matchingMessage {
			hook.Hook(config, TestFailure{
				StartTime: h.startTime,
				Message:   message,
			})
		}
	}
}

func New(name string, hooks ...Hook) *Handler {
	return &Handler{
		startTime: time.Now(),
		name:      name,
		hooks:     hooks,
	}
}
