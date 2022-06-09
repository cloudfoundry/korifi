package wiring

import (
	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/webhook"
	"code.cloudfoundry.org/lager"
	"github.com/pkg/errors"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func ResourceValidator(logger lager.Logger, manager manager.Manager, config eirinictrl.ControllerConfig) error {
	logger = logger.Session("resource-validator")

	decoder, err := admission.NewDecoder(scheme.Scheme)
	if err != nil {
		return errors.Wrap(err, "Failed to create admission decoder")
	}

	manager.GetWebhookServer().Register("/lrps", &admission.Webhook{
		Handler: webhook.NewLRPResourceValidator(logger, decoder),
	})

	return nil
}
