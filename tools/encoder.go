package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"

	"github.com/BooleanCat/go-functional/v2/it"
)

func EncodeValuesToSha224(values ...string) []string {
	return slices.Collect(it.Map(slices.Values(values), EncodeValueToSha224))
}

func EncodeValueToSha224(value string) string {
	hash := sha256.Sum224([]byte(value))
	return hex.EncodeToString(hash[:])
}
