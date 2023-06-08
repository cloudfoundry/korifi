package finalizer

//+kubebuilder:webhook:path=/mutate-korifi-cloudfoundry-org-v1alpha1-controllers-finalizer,mutating=true,failurePolicy=fail,sideEffects=None,groups=korifi.cloudfoundry.org,resources=cfapps;cfspaces;cfpackages;cforgs;cfroutes;cfdomains,verbs=create,versions=v1alpha1,name=mcffinalizer.korifi.cloudfoundry.org,admissionReviewVersions={v1,v1beta1}

import (
	"context"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type ControllersFinalizerWebhook struct {
	delegate *k8s.FinalizerWebhook
}

func NewControllersFinalizerWebhook() *ControllersFinalizerWebhook {
	return &ControllersFinalizerWebhook{
		delegate: k8s.NewFinalizerWebhook(map[string]k8s.FinalizerDescriptor{
			"CFApp":     {FinalizerName: korifiv1alpha1.CFAppFinalizerName, SetPolicy: k8s.Always},
			"CFSpace":   {FinalizerName: korifiv1alpha1.CFSpaceFinalizerName, SetPolicy: k8s.Always},
			"CFPackage": {FinalizerName: korifiv1alpha1.CFPackageFinalizerName, SetPolicy: k8s.Always},
			"CFOrg":     {FinalizerName: korifiv1alpha1.CFOrgFinalizerName, SetPolicy: k8s.Always},
			"CFRoute":   {FinalizerName: korifiv1alpha1.CFRouteFinalizerName, SetPolicy: k8s.Always},
			"CFDomain":  {FinalizerName: korifiv1alpha1.CFDomainFinalizerName, SetPolicy: k8s.Always},
		}),
	}
}

func (r *ControllersFinalizerWebhook) SetupWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register("/mutate-korifi-cloudfoundry-org-v1alpha1-controllers-finalizer", &admission.Webhook{
		Handler: r,
	})
	r.delegate.SetupWebhookWithManager(mgr)
}

func (r *ControllersFinalizerWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	return r.delegate.Handle(ctx, req)
}
