package repositories

type NotFoundError struct {
	Err          error
	ResourceType string
}

func (e NotFoundError) Error() string {
	msg := "not found"
	if e.ResourceType != "" {
		msg = e.ResourceType + " " + msg
	}
	if e.Err != nil {
		msg = msg + ": " + e.Err.Error()
	}
	return msg
}

func (e NotFoundError) Unwrap() error {
	return e.Err
}

type PermissionDeniedOrNotFoundError struct {
	Err error
}

func (e PermissionDeniedOrNotFoundError) Error() string {
	return "Resource not found or permission denied."
}

func (e PermissionDeniedOrNotFoundError) Unwrap() error {
	return e.Err
}

type ResourceNotFoundError struct {
	Err error
}

func (e ResourceNotFoundError) Error() string {
	return "Resource not found."
}

func (e ResourceNotFoundError) Unwrap() error {
	return e.Err
}
