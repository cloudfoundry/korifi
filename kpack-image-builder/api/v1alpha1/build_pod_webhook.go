package v1alpha1

import (
	"context"
	"fmt"

	"code.cloudfoundry.org/korifi/tools"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var logger = logf.Log.WithName("kpack-build-pod-security")

type PodSecurityAdder struct{}

func NewPodSecurityAdder() *PodSecurityAdder {
	return &PodSecurityAdder{}
}

func (a *PodSecurityAdder) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&corev1.Pod{}).
		WithDefaulter(a).
		Complete()
}

// Mutate path is found here: https://github.com/kubernetes-sigs/controller-runtime/blob/15154aaa67679df320008ed45534f83ff3d6922d/pkg/builder/webhook.go#L201-L204
//+kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=ignore,groups="",resources=pods,verbs=create,versions=v1,name=mkpackbuildpod.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1},sideEffects=none

var _ webhook.CustomDefaulter = &PodSecurityAdder{}

func (a *PodSecurityAdder) Default(ctx context.Context, obj runtime.Object) error {
	logger.Info("default", "name", obj.DeepCopyObject().GetObjectKind())

	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Pod but got a %T", obj))
	}

	logger.Info("patching build pod security context", "namespace", pod.Namespace, "name", pod.Name)
	patchContainerSecurity(pod.Spec.Containers)
	patchContainerSecurity(pod.Spec.InitContainers)

	return nil
}

func patchContainerSecurity(containers []corev1.Container) {
	for i := range containers {
		container := &containers[i]

		if container.SecurityContext == nil {
			container.SecurityContext = new(corev1.SecurityContext)
		}

		container.SecurityContext.AllowPrivilegeEscalation = tools.PtrTo(false)
		container.SecurityContext.RunAsNonRoot = tools.PtrTo(true)
		container.SecurityContext.Capabilities = &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		}
		container.SecurityContext.SeccompProfile = &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		}
	}
}
