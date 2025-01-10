package tools

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	CredentialsSecretKey = "credentials"
	ParametersSecretKey  = "parameters"
)

func ToCredentialsSecretData(credentials any) (map[string][]byte, error) {
	return toSecretData(CredentialsSecretKey, credentials)
}

func ToParametersSecretData(credentials any) (map[string][]byte, error) {
	return toSecretData(ParametersSecretKey, credentials)
}

func toSecretData(key string, value any) (map[string][]byte, error) {
	var valueBytes []byte
	valueBytes, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("failed to marshal secret value")
	}

	return map[string][]byte{
		key: valueBytes,
	}, nil
}

func FromCredentialsSecretData(data map[string][]byte) (map[string]any, error) {
	value, err := fromSecretData(CredentialsSecretKey, data)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials: %v", err)
	}
	return value, nil
}

func FromParametersSecretData(data map[string][]byte) (map[string]any, error) {
	value, err := fromSecretData(ParametersSecretKey, data)
	if err != nil {
		return nil, fmt.Errorf("failed to get parameters: %v", err)
	}
	return value, nil
}

func fromSecretData(key string, data map[string][]byte) (map[string]any, error) {
	var value map[string]any
	err := json.Unmarshal(data[key], &value)
	if err != nil {
		return nil, err
	}

	return value, nil
}
