package credentials

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

const ServiceBindingSecretTypePrefix = "servicebinding.io/"

func GetBindingSecretType(credentialsSecret *corev1.Secret) (corev1.SecretType, error) {
	credentials, err := getCredentials(credentialsSecret)
	if err != nil {
		return "", err
	}

	userProvidedType, isString := credentials["type"].(string)
	if isString {
		return corev1.SecretType(ServiceBindingSecretTypePrefix + userProvidedType), nil
	}

	return corev1.SecretType(ServiceBindingSecretTypePrefix + korifiv1alpha1.UserProvidedType), nil
}

func GetBindingSecretData(credentialsSecret *corev1.Secret) (map[string]string, error) {
	credentials, err := getCredentials(credentialsSecret)
	secretData := map[string]string{}

	for k, v := range credentials {
		valueString, ok := v.(string)
		if ok {
			secretData[k] = valueString
			continue
		}

		valueBytes, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal secret data value at key %q: %w", k, err)
		}
		secretData[k] = string(valueBytes)
	}

	return secretData, err
}

func GetCredentialsShaSum(credentialsSecret *corev1.Secret) (string, error) {
	credentialsBytes, err := getCredentialsRaw(credentialsSecret)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha1.Sum(credentialsBytes)), nil
}

func getCredentialsRaw(credentialsSecret *corev1.Secret) ([]byte, error) {
	credentials, ok := credentialsSecret.Data[korifiv1alpha1.CredentialsSecretKey]
	if !ok {
		return nil, fmt.Errorf(
			"data of secret %q does not contain the %q key",
			credentialsSecret.Name,
			korifiv1alpha1.CredentialsSecretKey,
		)
	}

	return credentials, nil
}

func getCredentials(credentialsSecret *corev1.Secret) (map[string]any, error) {
	credentials, err := getCredentialsRaw(credentialsSecret)
	if err != nil {
		return nil, err
	}
	credentialsObject := map[string]any{}
	err = json.Unmarshal(credentials, &credentialsObject)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal secret data: %w", err)
	}

	return credentialsObject, nil
}
