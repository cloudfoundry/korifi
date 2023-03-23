package handlers

import (
	"net/http"

	"code.cloudfoundry.org/korifi/api/errors"
	"code.cloudfoundry.org/korifi/api/routing"
)

func NotFound(r *http.Request) (*routing.Response, error) {
	return nil, errors.NewEndpointNotFoundError()
}
