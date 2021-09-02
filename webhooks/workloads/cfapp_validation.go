package workloads

import (
	"code.cloudfoundry.org/cf-k8s-controllers/apis/workloads/v1alpha1"
	"context"
	v1 "k8s.io/api/admission/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const specNameKey = "spec.name"

var cfapplog = logf.Log.WithName("cfapp-validate")

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-workloads-cloudfoundry-org-v1alpha1-cfapp,mutating=false,failurePolicy=fail,sideEffects=None,groups=workloads.cloudfoundry.org,resources=cfapps,verbs=create;update,versions=v1alpha1,name=vcfapp.kb.io,admissionReviewVersions={v1,v1beta1}

type CFAppValidation struct {
	Client  CFAppClient
	decoder *admission.Decoder
}

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . CFAppClient
type CFAppClient interface {
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error
}

func (v *CFAppValidation) SetupWebhookWithManager(mgr ctrl.Manager) error {
	//Register validate webhook endpoint with kubernetes manager
	mgr.GetWebhookServer().Register("/validate-workloads-cloudfoundry-org-v1alpha1-cfapp", &webhook.Admission{Handler: v})

	//Generate indexes for CFApp on field spec.name for efficient querying.
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &v1alpha1.CFApp{}, specNameKey,
		func(rawObj client.Object) []string {
			app := rawObj.(*v1alpha1.CFApp)
			return []string{app.Spec.Name}
		}); err != nil {
		return err
	}
	return nil
}

func (v *CFAppValidation) Handle(ctx context.Context, req admission.Request) admission.Response {
	cfapplog.Info("Validate", "name", req.Name)

	cfApp := v1alpha1.CFApp{}
	err := v.decoder.Decode(req, &cfApp)
	if err != nil {
		errMessage := "Error while decoding CFApp object"
		cfapplog.Error(err, errMessage)
		return admission.Denied(errMessage)
	}
	foundApps := &v1alpha1.CFAppList{}
	matchingFields := client.MatchingFields{specNameKey: cfApp.Spec.Name}
	err = v.Client.List(context.Background(), foundApps, client.InNamespace(cfApp.Namespace), matchingFields)
	if err != nil {
		errMessage := "Error while fetching CFApps using K8SClient"
		cfapplog.Error(err, errMessage)
		return admission.Denied(errMessage)
	}

	if req.Operation == v1.Create {
		if len(foundApps.Items) > 0 {
			errMessage := "CFApp with the same spec.name exists"
			cfapplog.Info(errMessage, "name", req.Name)
			return admission.Denied(errMessage)
		}
	} else if req.Operation == v1.Update {
		for _, foundCfApp := range foundApps.Items {
			if foundCfApp.Name != cfApp.Name {
				errMessage := "CFApp with the same spec.name exists"
				cfapplog.Info(errMessage, "name", req.Name)
				return admission.Denied(errMessage)
			}
		}
	}

	return admission.Allowed("")
}

// InjectDecoder injects the decoder.
func (v *CFAppValidation) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
