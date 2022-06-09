package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	eirinictrl "code.cloudfoundry.org/korifi/statefulset-runner"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/stset"
	"code.cloudfoundry.org/korifi/statefulset-runner/util"
	"code.cloudfoundry.org/lager"
	exterrors "github.com/pkg/errors"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type InstanceIndexEnvInjector struct {
	logger  lager.Logger
	decoder *admission.Decoder
}

func NewInstanceIndexEnvInjector(logger lager.Logger, decoder *admission.Decoder) *InstanceIndexEnvInjector {
	return &InstanceIndexEnvInjector{
		logger:  logger,
		decoder: decoder,
	}
}

func (i *InstanceIndexEnvInjector) Handle(ctx context.Context, req admission.Request) admission.Response {
	logger := i.logger.Session("handle-webhook-request")

	if req.Operation != admissionv1.Create {
		return admission.Allowed("pod was already created")
	}

	pod := &corev1.Pod{}

	err := i.decoder.Decode(req, pod)
	if err != nil {
		logger.Error("no-pod-in-request", err)

		return admission.Errored(http.StatusBadRequest, err)
	}

	logger = logger.WithData(lager.Data{"pod-name": pod.Name, "pod-namespace": pod.Namespace})

	podCopy := pod.DeepCopy()

	err = injectInstanceIndex(logger, podCopy)
	if err != nil {
		i.logger.Error("failed-to-inject-instance-index", err)

		return admission.Errored(http.StatusBadRequest, err)
	}

	marshaledPod, err := json.Marshal(podCopy)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

func injectInstanceIndex(logger lager.Logger, pod *corev1.Pod) error {
	index, err := util.ParseAppIndex(pod.Name)
	if err != nil {
		return exterrors.Wrap(err, "failed to parse app index")
	}

	for c := range pod.Spec.Containers {
		container := &pod.Spec.Containers[c]
		if container.Name == stset.ApplicationContainerName {
			cfInstanceVar := corev1.EnvVar{Name: eirinictrl.EnvCFInstanceIndex, Value: strconv.Itoa(index)}
			container.Env = append(container.Env, cfInstanceVar)

			logger.Debug("patching-instance-index", lager.Data{"env-var": cfInstanceVar})

			return nil
		}
	}

	logger.Info("no-app-container-found")

	return errors.New("no application container found in pod")
}
