package handlers_test

import (
	"code.cloudfoundry.org/korifi/api/handlers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = DescribeTable("UnauthenticatedEndpoint",
	func(requestPath string, expected bool) {
		endpoints := handlers.NewUnauthenticatedEndpoints()
		Expect(endpoints.IsUnauthenticatedEndpoint(requestPath)).To(Equal(expected))
	},
	Entry("/", "/", true),
	Entry("/v3", "/v3", true),
	Entry("/api/v1/info", "/api/v1/info", true),
	Entry("/oauth/token", "/oauth/token", true),
	Entry("/v3/apps", "/v3/apps", false),
)
