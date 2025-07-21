package repositories

import (
	"context"
	"fmt"
	"slices"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/repositories/k8sklient/descriptors"
	"github.com/BooleanCat/go-functional/v2/it"
)

type UserRecord struct {
	GUID string
	Name string
}

type ListUsersMessage struct {
	Names      []string
	Pagination Pagination
}

type UserRepository struct{}

func NewUserRepository() *UserRepository {
	return &UserRepository{}
}

func (r *UserRepository) ListUsers(ctx context.Context, _ authorization.Info, message ListUsersMessage) (ListResult[UserRecord], error) {
	userRecords := slices.Collect(it.Map(slices.Values(message.Names), func(name string) UserRecord {
		return UserRecord{GUID: name, Name: name}
	}))

	recordsPage := descriptors.SinglePage(userRecords, len(userRecords))
	if !message.Pagination.IsZero() {
		var err error
		recordsPage, err = descriptors.GetPage(userRecords, message.Pagination.PerPage, message.Pagination.Page)
		if err != nil {
			return ListResult[UserRecord]{}, fmt.Errorf("failed to page users list: %w", err)
		}
	}

	return ListResult[UserRecord]{
		PageInfo: recordsPage.PageInfo,
		Records:  recordsPage.Items,
	}, nil
}
