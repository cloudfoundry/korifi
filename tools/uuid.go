package tools

import (
	"strings"

	uuid "github.com/satori/go.uuid"
)

var korifiNs = uuid.NewV5(uuid.NamespaceDNS, "korifi.cloudfoundry.org")

func NamespacedUUID(segments ...string) string {
	return uuid.NewV5(korifiNs, strings.TrimSpace(strings.Join(segments, "::"))).String()
}
