package workloads

import "encoding/json"

type ValidationErrorCode int

type ValidationError struct {
	Code    ValidationErrorCode `json:"code"`
	Message string              `json:"message"`
}

const (
	UnknownError = ValidationErrorCode(iota)
	DuplicateAppError
)

func (w ValidationErrorCode) Marshal() string {
	bytes, err := json.Marshal(ValidationError{
		Code:    w,
		Message: w.GetMessage(),
	})
	if err != nil {
		return err.Error()
	}
	return string(bytes)
}

func (w *ValidationErrorCode) Unmarshall(payload string) {
	validationErr := new(ValidationError)
	err := json.Unmarshal([]byte(payload), validationErr)
	if err != nil {
		*w = UnknownError
	}
	*w = validationErr.Code
}

func (w ValidationErrorCode) GetMessage() string {
	switch w {
	case DuplicateAppError:
		return "CFApp with the same spec.name exists"
	default:
		return "An unknown error has occured"
	}
}
