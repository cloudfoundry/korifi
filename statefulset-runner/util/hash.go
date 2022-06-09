package util

// don't judge; we'll fix it later, we promise

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/pkg/errors"
)

const MaxHashLength = 10

func Hash(s string) (string, error) {
	sha := sha256.New()

	if _, err := sha.Write([]byte(s)); err != nil {
		return "", errors.Wrap(err, "failed to calculate sha")
	}

	hash := hex.EncodeToString(sha.Sum(nil))

	return hash[:MaxHashLength], nil
}
