package repositories

//go:generate controller-gen rbac:roleName=cf-admin-clusterrole paths=./... output:rbac:artifacts:config=../config/rbac

type NotFoundError struct {
	Err error
}

func (e NotFoundError) Error() string {
	return "not found"
}

func (e NotFoundError) Unwrap() error {
	return e.Err
}
