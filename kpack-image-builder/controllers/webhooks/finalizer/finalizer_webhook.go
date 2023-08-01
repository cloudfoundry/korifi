package finalizer

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-kpack-image-builder-finalizer,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org;kpack.io,resources=buildworkloads;builds,verbs=create,versions=v1alpha1;v1alpha2,name=mcf-kib-finalizer.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/kpack-image-builder/controllers"
	"code.cloudfoundry.org/korifi/tools/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type KpackImageBuilderFinalizerWebhook struct {
	delegate *k8s.FinalizerWebhook
}

func NewKpackImageBuilderFinalizerWebhook() *KpackImageBuilderFinalizerWebhook {
	return &KpackImageBuilderFinalizerWebhook{
		delegate: k8s.NewFinalizerWebhook(map[string]k8s.FinalizerDescriptor{
			"BuildWorkload": {FinalizerName: korifiv1alpha1.BuildWorkloadFinalizerName, SetPolicy: kpackImageBuilderBuildWorkloadsOnly},
			"Build":         {FinalizerName: controllers.KpackBuildFinalizer, SetPolicy: korifiBuildsOnly},
		}),
	}
}

func korifiBuildsOnly(obj unstructured.Unstructured) bool {
	_, hasBuildWorkloadLabel := obj.GetLabels()[controllers.BuildWorkloadLabelKey]
	return hasBuildWorkloadLabel
}

func kpackImageBuilderBuildWorkloadsOnly(obj unstructured.Unstructured) bool {
	var bw korifiv1alpha1.BuildWorkload
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &bw); err != nil {
		return false
	}

	return bw.Spec.BuilderName == controllers.KpackReconcilerName
}

func (r *KpackImageBuilderFinalizerWebhook) SetupWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register("/mutate-korifi-cloudfoundry-org-v1alpha1-kpack-image-builder-finalizer", &admission.Webhook{
		Handler: r,
	})
	r.delegate.SetupWebhookWithManager(mgr)
}

func (r *KpackImageBuilderFinalizerWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	return r.delegate.Handle(ctx, req)
}
