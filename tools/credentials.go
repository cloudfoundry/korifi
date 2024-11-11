package tools

import (
	"encoding/json"
	"errors"
)

const CredentialsSecretKey = "credentials"

func ToCredentialsSecretData(credentials any) (map[string][]byte, error) {
	var credentialBytes []byte
	credentialBytes, err := json.Marshal(credentials)
	if err != nil {
		return nil, errors.New("failed to marshal credentials for service instance")
	}

	return map[string][]byte{
		CredentialsSecretKey: credentialBytes,
	}, nil
}

func ToDecodedSecretDataCredentials(data map[string][]byte) (map[string]any, error) {
	var credentials map[string]any
	err := json.Unmarshal(data[CredentialsSecretKey], &credentials)
	if err != nil {
		return nil, errors.New("failed to unmarshal credentials for service instance")
	}

	return credentials, nil
}
