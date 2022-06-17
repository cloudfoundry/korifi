package webhooks

import (
	"context"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const AppDoesNotExistErrorType = "AppDoesNotExistError"

type AppExistsValidator struct {
	client client.Client
}

func NewAppExistsValidator(client client.Client) *AppExistsValidator {
	return &AppExistsValidator{client: client}
}

func (v *AppExistsValidator) EnsureCFApp(ctx context.Context, namespace, appGUID string) *ValidationError {
	err := v.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: appGUID}, &korifiv1alpha1.CFApp{})
	if err == nil {
		return nil
	}

	if k8serrors.IsNotFound(err) {
		return &ValidationError{
			Type:    AppDoesNotExistErrorType,
			Message: fmt.Sprintf("CFApp %s:%s does not exist", namespace, appGUID),
		}
	}

	return &ValidationError{
		Type:    UnknownErrorType,
		Message: UnknownErrorMessage,
	}
}
