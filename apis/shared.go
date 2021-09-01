package apis

import (
	"fmt"

	"code.cloudfoundry.org/cf-k8s-api/presenters"
)

func newNotFoundError(resourceName string) presenters.ErrorsResponse {
	return presenters.ErrorsResponse{Errors: []presenters.PresentedError{{
		Title:  fmt.Sprintf("%s not found", resourceName),
		Detail: "CF-ResourceNotFound",
		Code:   10010,
	}}}
}

func newUnknownError() presenters.ErrorsResponse {
	return presenters.ErrorsResponse{Errors: []presenters.PresentedError{{
		Title:  "UnknownError",
		Detail: "An unknown error occurred.",
		Code:   10001,
	}}}
}
