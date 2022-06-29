package podsecurity

import (
	"context"
	"encoding/json"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/mutate-kpack-build-pod,mutating=true,failurePolicy=ignore,groups="",resources=pods,verbs=create,versions=v1,name=mkpackbuildpod.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1},sideEffects=none

type PodSecurityAdder struct {
	decoder *admission.Decoder
}

func NewPodSecurityAdder() *PodSecurityAdder {
	return &PodSecurityAdder{}
}

func pointerTo(b bool) *bool {
	return &b
}

func (a *PodSecurityAdder) Handle(ctx context.Context, req admission.Request) admission.Response {
	logger := ctrl.Log.WithName("kpack-build-pod-security")

	pod := &corev1.Pod{}
	err := a.decoder.Decode(req, pod)
	if err != nil {
		logger.Error(err, "decode-error")
		return admission.Errored(http.StatusBadRequest, err)
	}

	patchContainerSecurity(pod.Spec.Containers)
	patchContainerSecurity(pod.Spec.InitContainers)

	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	logger.Info("patching pod security context", "namespace", req.Namespace, "name", req.Name)
	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

// InjectDecoder injects the decoder.
func (a *PodSecurityAdder) InjectDecoder(d *admission.Decoder) error {
	a.decoder = d
	return nil
}

func patchContainerSecurity(containers []corev1.Container) {
	for i := range containers {
		container := containers[i]
		if container.SecurityContext == nil {
			container.SecurityContext = new(corev1.SecurityContext)
		}

		container.SecurityContext.AllowPrivilegeEscalation = pointerTo(false)
		container.SecurityContext.RunAsNonRoot = pointerTo(true)
		container.SecurityContext.Capabilities = &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		}
		container.SecurityContext.SeccompProfile = &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		}
		containers[i] = container
	}
}
