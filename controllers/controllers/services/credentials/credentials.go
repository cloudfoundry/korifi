package credentials

import (
	"encoding/json"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
)

const ServiceBindingSecretTypePrefix = "servicebinding.io/"

func GetBindingSecretType(credentialsSecret *corev1.Secret) (corev1.SecretType, error) {
	credentials := map[string]any{}
	err := GetCredentials(credentialsSecret, &credentials)
	if err != nil {
		return "", err
	}

	userProvidedType, isString := credentials["type"].(string)
	if isString {
		return corev1.SecretType(ServiceBindingSecretTypePrefix + userProvidedType), nil
	}

	return corev1.SecretType(ServiceBindingSecretTypePrefix + korifiv1alpha1.UserProvidedType), nil
}

func GetServiceBindingIOSecretData(credentialsSecret *corev1.Secret) (map[string][]byte, error) {
	credentials := map[string]any{}
	err := GetCredentials(credentialsSecret, &credentials)
	if err != nil {
		return nil, err
	}
	secretData := map[string][]byte{}
	for k, v := range credentials {
		secretData[k], err = toBytes(v)
		if err != nil {
			return nil, fmt.Errorf("failed to convert value of key %q to bytes: %w", k, err)
		}
	}

	if _, hasType := secretData["type"]; !hasType {
		secretData["type"] = []byte("user-provided")
	}

	return secretData, err
}

func toBytes(value any) ([]byte, error) {
	valueString, ok := value.(string)
	if ok {
		return []byte(valueString), nil
	}

	return json.Marshal(value)
}

func GetCredentials(credentialsSecret *corev1.Secret, credentialsObject any) error {
	credentials, ok := credentialsSecret.Data[tools.CredentialsSecretKey]
	if !ok {
		return fmt.Errorf(
			"data of secret %q does not contain the %q key",
			credentialsSecret.Name,
			tools.CredentialsSecretKey,
		)
	}

	return errors.Wrap(json.Unmarshal(credentials, credentialsObject), "failed to unmarshal secret data")
}
