package stset

import (
	"context"
	"time"

	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/utils"
	"code.cloudfoundry.org/korifi/statefulset-runner/k8s/utils/dockerutils"
	eiriniv1 "code.cloudfoundry.org/korifi/statefulset-runner/pkg/apis/eirini/v1"
	"code.cloudfoundry.org/korifi/statefulset-runner/util"
	"code.cloudfoundry.org/lager"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

//counterfeiter:generate . LRPToStatefulSetConverter
//counterfeiter:generate . PodDisruptionBudgetUpdater

type LRPToStatefulSetConverter interface {
	Convert(statefulSetName string, lrp *eiriniv1.LRP, privateRegistrySecret *corev1.Secret) (*appsv1.StatefulSet, error)
}

type PodDisruptionBudgetUpdater interface {
	Update(ctx context.Context, stset *appsv1.StatefulSet, lrp *eiriniv1.LRP) error
}

type Desirer struct {
	logger                     lager.Logger
	lrpToStatefulSetConverter  LRPToStatefulSetConverter
	podDisruptionBudgetCreator PodDisruptionBudgetUpdater
	scheme                     *runtime.Scheme
	client                     client.Client
}

func NewDesirer(
	logger lager.Logger,
	lrpToStatefulSetConverter LRPToStatefulSetConverter,
	podDisruptionBudgetCreator PodDisruptionBudgetUpdater,
	client client.Client,
	scheme *runtime.Scheme,
) *Desirer {
	return &Desirer{
		logger:                     logger,
		lrpToStatefulSetConverter:  lrpToStatefulSetConverter,
		podDisruptionBudgetCreator: podDisruptionBudgetCreator,
		client:                     client,
		scheme:                     scheme,
	}
}

func (d *Desirer) Desire(ctx context.Context, lrp *eiriniv1.LRP) error {
	logger := d.logger.Session("desire", lager.Data{"guid": lrp.Spec.GUID, "version": lrp.Spec.Version, "namespace": lrp.Namespace})

	statefulSetName, err := utils.GetStatefulsetName(lrp)
	if err != nil {
		return err
	}

	privateRegistrySecret, err := d.createRegistryCredsSecretIfRequired(ctx, lrp)
	if err != nil {
		return err
	}

	st, err := d.lrpToStatefulSetConverter.Convert(statefulSetName, lrp, privateRegistrySecret)
	if err != nil {
		return err
	}

	st.Namespace = lrp.Namespace

	if err = ctrl.SetControllerReference(lrp, st, d.scheme); err != nil {
		return errors.Wrap(err, "failed to set controller reference")
	}

	err = d.client.Create(ctx, st)
	if err != nil {
		var statusErr *k8serrors.StatusError
		if errors.As(err, &statusErr) && statusErr.Status().Reason == metav1.StatusReasonAlreadyExists {
			logger.Debug("statefulset-already-exists", lager.Data{"error": err.Error()})

			return nil
		}

		return d.cleanupAndError(ctx, errors.Wrap(err, "failed to create statefulset"), privateRegistrySecret)
	}

	if err := d.setSecretOwner(ctx, privateRegistrySecret, st); err != nil {
		logger.Error("failed-to-set-owner-to-the-registry-secret", err)

		return errors.Wrap(err, "failed to set owner to the registry secret")
	}

	if err := d.podDisruptionBudgetCreator.Update(ctx, st, lrp); err != nil {
		logger.Error("failed-to-create-pod-disruption-budget", err)

		return errors.Wrap(err, "failed to create pod disruption budget")
	}

	return nil
}

func (d *Desirer) setSecretOwner(ctx context.Context, privateRegistrySecret *corev1.Secret, stSet *appsv1.StatefulSet) error {
	if privateRegistrySecret == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	originalSecret := privateRegistrySecret.DeepCopy()

	if err := controllerutil.SetOwnerReference(stSet, privateRegistrySecret, scheme.Scheme); err != nil {
		return errors.Wrap(err, "secret-client-set-owner-ref-failed")
	}

	return d.client.Patch(ctx, privateRegistrySecret, client.MergeFrom(originalSecret))
}

func (d *Desirer) createRegistryCredsSecretIfRequired(ctx context.Context, lrp *eiriniv1.LRP) (*corev1.Secret, error) {
	if lrp.Spec.PrivateRegistry == nil {
		return nil, nil // nolint: nilnil
	}

	secret, err := generateRegistryCredsSecret(lrp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate private registry secret for statefulset")
	}

	err = d.client.Create(ctx, secret)

	return secret, errors.Wrap(err, "failed to create private registry secret for statefulset")
}

func (d *Desirer) cleanupAndError(ctx context.Context, stsetCreationError error, privateRegistrySecret *corev1.Secret) error {
	resultError := multierror.Append(nil, stsetCreationError)

	if privateRegistrySecret != nil {
		err := d.client.Delete(ctx, privateRegistrySecret)
		if err != nil {
			resultError = multierror.Append(resultError, errors.Wrap(err, "failed to cleanup registry secret"))
		}
	}

	return resultError
}

func generateRegistryCredsSecret(lrp *eiriniv1.LRP) (*corev1.Secret, error) {
	dockerConfig := dockerutils.NewDockerConfig(
		util.ParseImageRegistryHost(lrp.Spec.Image),
		lrp.Spec.PrivateRegistry.Username,
		lrp.Spec.PrivateRegistry.Password,
	)

	dockerConfigJSON, err := dockerConfig.JSON()
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate privete registry config")
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    lrp.Namespace,
			GenerateName: PrivateRegistrySecretGenerateName,
		},
		Type: corev1.SecretTypeDockerConfigJson,
		StringData: map[string]string{
			dockerutils.DockerConfigKey: dockerConfigJSON,
		},
	}, nil
}
