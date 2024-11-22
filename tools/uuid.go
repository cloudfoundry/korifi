package tools

import (
	"strings"

	uuid "github.com/satori/go.uuid"
)

func NamespacedUUID(ns string, segments ...string) string {
	return uuid.NewV5(uuid.NewV5(uuid.NamespaceDNS, ns), strings.TrimSpace(strings.Join(segments, ":"))).String()
}
