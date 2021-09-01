package repositories

type NotFoundError struct {
	Err error
}

func (e NotFoundError) Error() string {
	return "not found"
}

func (e NotFoundError) Unwrap() error {
	return e.Err
}
