package tools

import (
	"strings"

	uuid "github.com/satori/go.uuid"
)

var korifiNs = uuid.NewV5(uuid.NamespaceDNS, "korifi.cloudfoundry.org")

func UUID(input string) string {
	return uuid.NewV5(korifiNs, strings.TrimSpace(input)).String()
}
