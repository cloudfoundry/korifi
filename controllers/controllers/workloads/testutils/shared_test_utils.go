package testutils

import (
	"github.com/google/uuid"
)

func PrefixedGUID(prefix string) string {
	return prefix + "-" + uuid.NewString()[:8]
}
